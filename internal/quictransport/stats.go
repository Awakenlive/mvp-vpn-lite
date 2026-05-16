package quictransport

import "mvp-vpn-lite/internal/stats"

func statsSnapshotFormatter(jsonOutput bool) stats.SnapshotFormatter {
	if jsonOutput {
		return stats.JSONSnapshot
	}
	return stats.TextSnapshot
}
