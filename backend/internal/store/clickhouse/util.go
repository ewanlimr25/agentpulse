package clickhouse

import "time"

func nsToTime(ns int64) time.Time {
	return time.Unix(0, ns).UTC()
}
