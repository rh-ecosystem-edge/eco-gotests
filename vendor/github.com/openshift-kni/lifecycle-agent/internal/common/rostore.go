/*
Copyright 2026.

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

package common

import "fmt"

// roDurabilityCrioDropInTemplate is the deterministic CRI-O drop-in written by self-contained
// USB IBI prepare. It is centralized here so prepare and post-pivot validate the exact same
// content instead of each package independently reconstructing it.
const roDurabilityCrioDropInTemplate = `# Written by lca-cli ibi prepare for self-contained USB-based IBI media.
#
# Registers the on-disk, read-only containers/storage durability store copied from the installer
# media as a read-only additional image store, so the installed node resolves the digest-pinned
# image closure locally with no registry and no network. The copy survives kubelet GC and CRI-O
# reboot-time cleanup because both act only on the writable graphroot.
[crio]
storage_option = [
  "imagestore=%s",
]
`

// RODurabilityCrioDropInContent returns the exact CRI-O drop-in content used to register the
// persisted read-only additional image store.
func RODurabilityCrioDropInContent() string {
	return fmt.Sprintf(roDurabilityCrioDropInTemplate, RODurabilityStorePath)
}
