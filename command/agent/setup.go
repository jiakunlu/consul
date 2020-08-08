package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/hashicorp/consul/agent"
	autoconf "github.com/hashicorp/consul/agent/auto-config"
	"github.com/hashicorp/consul/agent/cache"
	certmon "github.com/hashicorp/consul/agent/cert-monitor"
	"github.com/hashicorp/consul/agent/config"
	"github.com/hashicorp/consul/agent/pool"
	"github.com/hashicorp/consul/agent/structs"
	"github.com/hashicorp/consul/agent/token"
	"github.com/hashicorp/consul/ipaddr"
	"github.com/hashicorp/consul/lib"
	"github.com/hashicorp/consul/logging"
	"github.com/hashicorp/consul/tlsutil"
	"github.com/hashicorp/go-hclog"
)

// TODO: maybe move this back into agent package to make TestAgent work
// TODO: store options on a different struct, not cmd, and use that as an arg
func newBaseDeps(cmd *cmd, out io.Writer) (agent.BaseDeps, error) {
	d := agent.BaseDeps{}
	cfg, warnings, err := config.Load(cmd.flagArgs, config.Source{})
	if err != nil {
		return d, err
	}
	for _, w := range warnings {
		cmd.UI.Warn(w)
	}

	// TODO: move config printout here (from run) to remove the need for the gatedUI

	// TODO: use logging.Config in RuntimeConfig instead of separate fields
	logConf := &logging.Config{
		LogLevel:          cfg.LogLevel,
		LogJSON:           cfg.LogJSON,
		Name:              logging.Agent,
		EnableSyslog:      cfg.EnableSyslog,
		SyslogFacility:    cfg.SyslogFacility,
		LogFilePath:       cfg.LogFile,
		LogRotateDuration: cfg.LogRotateDuration,
		LogRotateBytes:    cfg.LogRotateBytes,
		LogRotateMaxFiles: cfg.LogRotateMaxFiles,
	}
	d.Logger, err = logging.Setup(logConf, []io.Writer{out})
	if err != nil {
		return d, err
	}

	cfg.NodeID, err = newNodeIDFromConfig(cfg, d.Logger)
	if err != nil {
		return d, fmt.Errorf("failed to setup node ID: %w", err)
	}

	d.TelemetrySink, err = lib.InitTelemetry(cfg.Telemetry)
	if err != nil {
		return d, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	d.TLSConfigurator, err = tlsutil.NewConfigurator(cfg.ToTLSUtilConfig(), d.Logger)
	if err != nil {
		return d, err
	}

	d.RuntimeConfig = cfg
	d.Tokens = new(token.Store)
	// cache-types are not registered yet, but they won't be used until the components are started.
	d.Cache = cache.New(cfg.Cache)
	d.ConnPool = newConnPool(cfg, d.Logger, d.TLSConfigurator)

	deferredAC := &deferredAutoConfig{}

	cmConf := new(certmon.Config).
		WithCache(d.Cache).
		WithTLSConfigurator(d.TLSConfigurator).
		WithDNSSANs(cfg.AutoConfig.DNSSANs).
		WithIPSANs(cfg.AutoConfig.IPSANs).
		WithDatacenter(cfg.Datacenter).
		WithNodeName(cfg.NodeName).
		WithFallback(deferredAC.autoConfigFallbackTLS).
		WithLogger(d.Logger.Named(logging.AutoConfig)).
		WithTokens(d.Tokens).
		WithPersistence(deferredAC.autoConfigPersist)
	acCertMon, err := certmon.New(cmConf)
	if err != nil {
		return d, err
	}

	acConf := autoconf.Config{
		DirectRPC:   d.ConnPool,
		Logger:      d.Logger,
		CertMonitor: acCertMon,
		Loader: func(source config.Source) (*config.RuntimeConfig, []string, error) {
			return config.Load(cmd.flagArgs, source)
		},
	}
	ac, err := autoconf.New(acConf)
	if err != nil {
		return d, err
	}
	// TODO: can this cyclic dependency be un-cycled?
	deferredAC.autoConf = ac

	return d, nil
}

func newConnPool(config *config.RuntimeConfig, logger hclog.Logger, tls *tlsutil.Configurator) *pool.ConnPool {
	var rpcSrcAddr *net.TCPAddr
	if !ipaddr.IsAny(config.RPCBindAddr) {
		rpcSrcAddr = &net.TCPAddr{IP: config.RPCBindAddr.IP}
	}

	pool := &pool.ConnPool{
		Server:          config.ServerMode,
		SrcAddr:         rpcSrcAddr,
		Logger:          logger.StandardLogger(&hclog.StandardLoggerOptions{InferLevels: true}),
		TLSConfigurator: tls,
		Datacenter:      config.Datacenter,
	}
	if config.ServerMode {
		pool.MaxTime = 2 * time.Minute
		pool.MaxStreams = 64
	} else {
		pool.MaxTime = 127 * time.Second
		pool.MaxStreams = 32
	}
	return pool
}

type deferredAutoConfig struct {
	autoConf *autoconf.AutoConfig // TODO: use an interface
}

func (a *deferredAutoConfig) autoConfigFallbackTLS(ctx context.Context) (*structs.SignedResponse, error) {
	if a.autoConf == nil {
		return nil, fmt.Errorf("AutoConfig manager has not been created yet")
	}
	return a.autoConf.FallbackTLS(ctx)
}

func (a *deferredAutoConfig) autoConfigPersist(resp *structs.SignedResponse) error {
	if a.autoConf == nil {
		return fmt.Errorf("AutoConfig manager has not been created yet")
	}
	return a.autoConf.RecordUpdatedCerts(resp)
}
