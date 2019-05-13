/*
 * Copyright 2019 The NATS Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package core

import (
	"time"
)

// ShortKey returns the first 12 characters of a public key (or the key if it is < 12 long)
func ShortKey(s string) string {
	if s != "" && len(s) > 12 {
		s = s[0:12]
	}

	return s
}

// UnixToDate parses a unix date in UTC to a time
func UnixToDate(d int64) string {
	if d == 0 {
		return ""
	}
	when := time.Unix(d, 0).UTC()
	return when.Format("2006-01-02")
}
