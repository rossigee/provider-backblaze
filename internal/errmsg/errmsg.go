/*
Copyright 2025 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package errmsg provides a way to render an error as a stable, deterministic
// string suitable for use in status condition messages. It strips volatile
// per-request identifiers so the same failure renders identically on every
// reconcile, letting the status update be skipped when nothing changed.
package errmsg

import (
	"regexp"
	"strings"
)

var (
	requestID  = regexp.MustCompile(`[\s,]*RequestID:\s*[0-9a-fA-F\-]{8,}`)
	hostID     = regexp.MustCompile(`[\s,]*HostID:\s*[A-Za-z0-9+/=_-]{8,}`)
	emptyParts = regexp.MustCompile(`,(\s*,)+`)
	multiSpace = regexp.MustCompile(`\s{2,}`)
)

// Stable returns a deterministic form of errMsg suitable for use as a
// condition message. Request identifiers emitted by the Backblaze B2 API
// (RequestID, HostID) are stripped, because they differ on every call and
// would otherwise defeat change detection and re-trigger the controller's
// watch. Remaining text is whitespace-normalised and the result is trimmed.
func Stable(errMsg string) string {
	s := requestID.ReplaceAllString(errMsg, "")
	s = hostID.ReplaceAllString(s, "")
	s = emptyParts.ReplaceAllString(s, ",")
	s = multiSpace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	// Stripping an identifier at the start of the message can leave a
	// dangling leading separator.
	return strings.TrimPrefix(s, ", ")
}
