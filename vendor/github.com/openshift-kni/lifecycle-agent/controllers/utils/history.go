package utils

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	ibuv1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
	ipcv1 "github.com/openshift-kni/lifecycle-agent/api/ipconfig/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResetHistory reset the .status.history by setting the list to empty
func ResetHistory(client client.Client, log logr.Logger, ibu *ibuv1.ImageBasedUpgrade) {
	if ibu.Spec.Stage == ibuv1.Stages.Idle {
		ibu.Status.History = []*ibuv1.History{}
		updateStatus(client, log, ibu)
	}
}

// StartStageHistory this is called before a stage handler is called for the first time,
// which starts a timer know how long a stage took to complete.
// Time timer stopped when the stage completes successfully using StopStageHistory
func StartStageHistory(client client.Client, log logr.Logger, ibu *ibuv1.ImageBasedUpgrade) {
	if ibu.Spec.Stage == ibuv1.Stages.Idle {
		return
	}

	curHistory := ibu.Status.History
	for _, h := range curHistory {
		if h.Stage == ibu.Spec.Stage && !h.StartTime.IsZero() {
			return // stage in progress
		}
	}

	newHistoryEntry := ibuv1.History{
		Stage:     ibu.Spec.Stage,
		StartTime: getMetav1Now(),
		Phases:    []*ibuv1.Phase{},
	}
	ibu.Status.History = append(ibu.Status.History, &newHistoryEntry)
	updateStatus(client, log, ibu)
}

// StopStageHistory call when a stage completes successfully. This is a no op unless StartStageHistory is called first
func StopStageHistory(client client.Client, log logr.Logger, ibu *ibuv1.ImageBasedUpgrade) {
	curHistory := ibu.Status.History
	for _, h := range curHistory {
		if h.Stage == ibu.Spec.Stage && !h.StartTime.IsZero() && h.CompletionTime.IsZero() {
			// double check and warn in case a phase was not closed properly
			for _, p := range h.Phases {
				if p.CompletionTime.IsZero() {
					log.Info("WARNING: phase CompletionTime should be updated to a non zero value before its stage CompletionTime", "phase", p.Phase, "stage", ibu.Spec.Stage)
				}
			}
			h.CompletionTime = getMetav1Now()
			ibu.Status.History = curHistory
			updateStatus(client, log, ibu)
		}
	}
}

// StartPhase This can be called after StartStageHistory to allow for a more granular view
// of the important phases that are performed when moving a desired Stage.
// Phase timer is stopped when the phase completes successfully using StopPhase
func StartPhase(client client.Client, log logr.Logger, ibu *ibuv1.ImageBasedUpgrade, phase string) {
	curHistory := ibu.Status.History
	for _, h := range curHistory {
		if h.Stage == ibu.Spec.Stage {
			for _, p := range h.Phases {
				if p.Phase == phase && !p.StartTime.IsZero() {
					return // phase in progress
				}
			}
		}
	}

	for _, h := range curHistory {
		if h.Stage == ibu.Spec.Stage {
			newPhase := ibuv1.Phase{
				Phase:     phase,
				StartTime: getMetav1Now(),
			}
			h.Phases = append(h.Phases, &newPhase)
			updateStatus(client, log, ibu)
		}
	}
}

// StopPhase call when a phase completes successfully. This is a no op unless StartPhase is called first
func StopPhase(client client.Client, log logr.Logger, ibu *ibuv1.ImageBasedUpgrade, phase string) {
	curHistory := ibu.Status.History
	for _, h := range curHistory {
		if h.Stage == ibu.Spec.Stage {
			for _, p := range h.Phases {
				if p.Phase == phase && !p.StartTime.IsZero() && p.CompletionTime.IsZero() {
					p.CompletionTime = getMetav1Now()
					updateStatus(client, log, ibu)
				}
			}
		}
	}
}

// A helper function to return the current time. This also used to override time during tests
var getMetav1Now = func() metav1.Time {
	return metav1.Time{Time: time.Now()}
}

func updateStatus(client client.Client, log logr.Logger, ibu *ibuv1.ImageBasedUpgrade) {
	if err := client.Status().Update(context.Background(), ibu); err != nil {
		log.Error(err, "failed to update status with history info")
	}
}

// ResetIPHistory resets the IPConfig .status.history by setting the list to empty when stage is Idle
func ResetIPHistory(client client.Client, log logr.Logger, ipc *ipcv1.IPConfig) {
	if ipc.Spec.Stage == ipcv1.IPStages.Idle {
		ipc.Status.History = []*ipcv1.IPHistory{}
		updateIPStatus(client, log, ipc)
	}
}

// StartIPStageHistory starts a timer for the current IPConfig stage.
// Timer is stopped when the stage completes successfully using StopIPStageHistory
func StartIPStageHistory(client client.Client, log logr.Logger, ipc *ipcv1.IPConfig) {
	if ipc.Spec.Stage == ipcv1.IPStages.Idle {
		return
	}

	curHistory := ipc.Status.History
	for _, h := range curHistory {
		if h.Stage == ipc.Spec.Stage && !h.StartTime.IsZero() {
			return // stage in progress
		}
	}

	newHistoryEntry := ipcv1.IPHistory{
		Stage:     ipc.Spec.Stage,
		StartTime: getMetav1Now(),
		Phases:    []*ipcv1.IPPhase{},
	}
	ipc.Status.History = append(ipc.Status.History, &newHistoryEntry)
	updateIPStatus(client, log, ipc)
}

// StopIPStageHistory marks the current IPConfig stage as completed successfully.
// This is a no-op unless StartIPStageHistory was called first
func StopIPStageHistory(client client.Client, log logr.Logger, ipc *ipcv1.IPConfig) {
	curHistory := ipc.Status.History
	for _, h := range curHistory {
		if h.Stage == ipc.Spec.Stage && !h.StartTime.IsZero() && h.CompletionTime.IsZero() {
			// double check and warn in case a phase was not closed properly
			for _, p := range h.Phases {
				if p.CompletionTime.IsZero() {
					log.Info(
						"WARNING: phase CompletionTime should be updated to a non zero value before its stage CompletionTime",
						"phase", p.Phase,
						"stage", ipc.Spec.Stage,
					)
				}
			}
			h.CompletionTime = getMetav1Now()
			ipc.Status.History = curHistory
			updateIPStatus(client, log, ipc)
		}
	}
}

// StartIPPhase adds a phase entry for the current stage and marks its start time.
// Call StopIPPhase when the phase completes successfully
func StartIPPhase(client client.Client, log logr.Logger, ipc *ipcv1.IPConfig, phase string) {
	curHistory := ipc.Status.History
	for _, h := range curHistory {
		if h.Stage == ipc.Spec.Stage {
			for _, p := range h.Phases {
				if p.Phase == phase && !p.StartTime.IsZero() {
					return // phase in progress
				}
			}
		}
	}

	for _, h := range curHistory {
		if h.Stage == ipc.Spec.Stage {
			newPhase := ipcv1.IPPhase{
				Phase:     phase,
				StartTime: getMetav1Now(),
			}
			h.Phases = append(h.Phases, &newPhase)
			updateIPStatus(client, log, ipc)
		}
	}
}

// StopIPPhase marks a phase as completed successfully. This is a no-op unless StartIPPhase was called first
func StopIPPhase(client client.Client, log logr.Logger, ipc *ipcv1.IPConfig, phase string) {
	curHistory := ipc.Status.History
	for _, h := range curHistory {
		if h.Stage == ipc.Spec.Stage {
			for _, p := range h.Phases {
				if p.Phase == phase && !p.StartTime.IsZero() && p.CompletionTime.IsZero() {
					p.CompletionTime = metav1.Time{Time: getMetav1Now().Time}
					updateIPStatus(client, log, ipc)
				}
			}
		}
	}
}

func updateIPStatus(client client.Client, log logr.Logger, ipc *ipcv1.IPConfig) {
	if err := client.Status().Update(context.Background(), ipc); err != nil {
		log.Error(err, "failed to update ipconfig status with history info")
	}
}
