package application_context

import (
	"mahresources/download_queue"
	"mahresources/plugin_system"
)

func newDownloadThrottleResolver(pm *plugin_system.PluginManager) download_queue.ThrottleResolver {
	return func(pluginName string) (download_queue.DomainPolicy, bool) {
		if pm == nil || pluginName == "" {
			return download_queue.DomainPolicy{}, false
		}
		limits, ok := pm.DownloadLimitsForPlugin(pluginName)
		if !ok || len(limits) == 0 {
			return download_queue.DomainPolicy{}, false
		}

		rules := make([]download_queue.DomainRule, 0, len(limits))
		for _, limit := range limits {
			limit := limit
			rules = append(rules, download_queue.DomainRule{
				Key:         limit.Host,
				Concurrency: limit.Concurrency,
				MinInterval: limit.MinInterval,
				Backoff:     limit.Backoff,
				Match:       limit.Matches,
			})
		}
		return download_queue.DomainPolicy{Rules: rules}, true
	}
}
