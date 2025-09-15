package utils

import "time"

const IsoDateTimeLayout = "2006-01-02T15:04:05.000-0700"

func ParseIsoTimestamp(isoTimestamp string) (time.Time, error) {
	return time.Parse(IsoDateTimeLayout, isoTimestamp)
}
