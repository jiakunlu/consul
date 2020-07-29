package agent

import (
	"github.com/hashicorp/consul/agent/config"
	"github.com/hashicorp/consul/agent/consul"
)

type DelegateConfig struct {
	consulConfig *consul.Config
	options      []consul.ConsulOption
}

func setupDelegate(opts []consul.ConsulOption, c *config.RuntimeConfig) (delegate, []component, error) {
	/*
		if !c.ServerMode {
			client, err := consul.NewClientWithOptions(consulCfg, opts...)
			if err != nil {
				return nil, nil, fmt.Errorf("Failed to start Consul client: %v", err)
			}
			return client, nil, nil
		}

		comps, serverOpts := newServerComponents(consulCfg)
		options = append(options, serverOpts...)
		a.components = append(a.components, comps...)

		server, err := consul.NewServer(consulCfg, options...)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to start Consul server: %v", err)
		}
		return server, comps, nil
	*/
	return nil, nil, nil
}
