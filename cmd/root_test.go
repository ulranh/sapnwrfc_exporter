package cmd_test

import "github.com/ulranh/sapnwrfc_exporter/cmd"

func getTestConfig(mCnt, sCnt int) *cmd.Config {
	// mi := []cmd.MetricInfo{
	// 	cmd.MetricInfo{
	// 		Name:       "m1",
	// 		Help:       "h1",
	// 		MetricType: "gauge",
	// 		// SQL:          "select count(*) from <SCHEMA>.m_blocked_transactions",
	// 		SchemaFilter: []string{"sys"},
	// 	},
	// 	cmd.MetricInfo{
	// 		Name:       "m2",
	// 		Help:       "h2",
	// 		MetricType: "gauge",
	// 		// SQL:          "select allocated_size,port from <SCHEMA>.m_rs_memory where category='TABLE'",
	// 		SchemaFilter: []string{"sys"},
	// 	},
	// 	cmd.MetricInfo{
	// 		Name:       "m3",
	// 		Help:       "h3",
	// 		MetricType: "gauge",
	// 		// SQL:        "select top 1 (case when active_status = 'YES' then 1 else -1 end), database_name from <SCHEMA>.m_databases",
	// 		TagFilter: []string{"erp"},
	// 	},
	// 	cmd.MetricInfo{
	// 		Name:       "m4",
	// 		Help:       "h4",
	// 		MetricType: "gauge",
	// 		// SQL:        "update",
	// 	},
	// }

	si := []cmd.SystemInfo{
		cmd.SystemInfo{
			Name: "d01",
		},
		cmd.SystemInfo{
			Name: "D02",
		},
		cmd.SystemInfo{
			Name: "d03",
			Tags: []string{"bw"},
		},
	}
	config := cmd.Config{
		// Metrics: mi[:mCnt],
		Systems: si[:sCnt],
		Timeout: 3,
	}
	return &config
}
