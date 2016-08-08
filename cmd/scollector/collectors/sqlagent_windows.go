package collectors

import (
	"strings"
	"time"

	"bosun.org/metadata"
	"bosun.org/opentsdb"
	"bosun.org/slog"
	"github.com/StackExchange/wmi"
)

func init() {
	c := &IntervalCollector{
		F:        c_mssql_agents,
		Interval: time.Minute * 5,
	}
	c.init = wmiInit(c, func() interface{} { return &[]Win32_Service{} }, wqlSQLAgentInstanceFilter, &sqlAgentQuery)
	collectors = append(collectors, c)
}

const (
	wqlSQLAgentInstanceFilter string = `WHERE (Name Like 'SQLAgent$%' or Name = 'SQLSERVERAGENT') and not (StartMode = 'Disabled')`
)

var (
	sqlAgentQuery string
)

func c_mssql_agents() (opentsdb.MultiDataPoint, error) {
	var err error
	var svc_dst []Win32_Service
	var svc_q = wmi.CreateQuery(&svc_dst, wqlSQLAgentInstanceFilter)
	err = queryWmi(svc_q, &svc_dst)
	if err != nil {
		return nil, slog.Wrap(err)
	}
	var md opentsdb.MultiDataPoint
	add := func(f func([]Win32_Service) (opentsdb.MultiDataPoint, error)) {
		dps, e := f(svc_dst)
		if e != nil {
			err = e
		}
		md = append(md, dps...)
	}
	add(c_mssql_agent)
	return md, err
}

func c_mssql_agent(svc_dst []Win32_Service) (opentsdb.MultiDataPoint, error) {
	var md opentsdb.MultiDataPoint
	for _, w := range svc_dst {
		var dst []Win32_PerfRawData_SQLSERVERAGENT_SQLAgentJobs
		//Default Instance: Win32_PerfRawData_SQLSERVERAGENT_SQLAgentJobs                  Service: SQLSERVERAGENT
		//Named Instance:   Win32_PerfRawData_SQLAgentOSCARMAYER_SQLAgentOSCARMAYERJobs    Service: SQLAgent$OSCARMAYER
		//WMI Class has Alerts, Schedules, and Other instances, but for now we'll just collect the totals
		q := wmi.CreateQuery(&dst, `WHERE Name = '_Total'`)
		label := "mssqlserver"
		if w.Name != `SQLSERVERAGENT` {
			q = instanceAgentWMIQuery(w.Name, q)
			label = strings.ToLower(w.Name[8:len(w.Name)])
		}
		if err := queryWmi(q, &dst); err != nil {
			return nil, slog.Wrap(err)
		}
		for _, v := range dst {
			tags := opentsdb.TagSet{"instance": label}
			Add(&md, "mssql.agent.jobs", v.Activejobs, opentsdb.TagSet{"type": "active"}.Merge(tags), metadata.Counter, metadata.Count, descMSSQLAgentActivejobs)
			Add(&md, "mssql.agent.jobs", v.Failedjobs, opentsdb.TagSet{"type": "failed"}.Merge(tags), metadata.Counter, metadata.Count, descMSSQLAgentFailedjobs)
			Add(&md, "mssql.agent.jobs", v.Successfuljobs, opentsdb.TagSet{"type": "successful"}.Merge(tags), metadata.Counter, metadata.Count, descMSSQLAgentSuccessfuljobs)
		}
	}
	return md, nil
}

const (
	descMSSQLAgentActivejobs     = "Number of running jobs."
	descMSSQLAgentFailedjobs     = "Number of failed jobs."
	descMSSQLAgentSuccessfuljobs = "Number of successful jobs."
)

type Win32_PerfRawData_SQLSERVERAGENT_SQLAgentJobs struct {
	Activejobs     uint64
	Failedjobs     uint64
	Successfuljobs uint64
}

func instanceAgentWMIQuery(instancename string, wmiquery string) string {
	var newname = strings.Replace(strings.Replace(instancename, `$`, "", 1), `_`, "", -1)
	return strings.Replace(wmiquery, `SQLSERVERAGENT_SQLAgent`, newname+`_`+newname, 1)
}
