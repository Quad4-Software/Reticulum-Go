// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"fmt"
	"math"
	"time"
)

func parseRequestedAt(v any) (time.Time, error) {
	switch t := v.(type) {
	case float64:
		sec := int64(t)
		nsec := int64(math.Round((t - float64(sec)) * float64(time.Second)))
		return time.Unix(sec, nsec), nil
	case int64:
		return time.Unix(t, 0), nil
	case int:
		return time.Unix(int64(t), 0), nil
	case int32:
		return time.Unix(int64(t), 0), nil
	case uint64:
		return time.Unix(int64(t), 0), nil
	case uint32:
		return time.Unix(int64(t), 0), nil
	default:
		return time.Time{}, fmt.Errorf("invalid requested_at type: %T", v)
	}
}

func requestTimestampValid(requestedAt time.Time, now time.Time) bool {
	pastLimit := now.Add(-time.Duration(RequestTimestampMaxSkewPast) * time.Second)
	futureLimit := now.Add(time.Duration(RequestTimestampMaxSkewFuture) * time.Second)
	return !requestedAt.Before(pastLimit) && !requestedAt.After(futureLimit)
}
