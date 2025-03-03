package resource

// Represents a stage in the Developer Journey lifecycle.
type LifecycleStage int

const (
	LifecycleStageCode    LifecycleStage = iota + 1 // code
	LifecycleStageBuild                             // build
	LifecycleStageTest                              // test
	LifecycleStageRelease                           // release
	LifecycleStageDeploy                            // deploy
	LifecycleStageOperate                           // operate
	LifecycleStageMonitor                           // monitor
	LifecycleStageOther                             // other
)

// String returns the string representation of the LifecycleStage.
func (l LifecycleStage) String() string {
	switch l {
	case LifecycleStageCode:
		return "code"
	case LifecycleStageBuild:
		return "build"
	case LifecycleStageTest:
		return "test"
	case LifecycleStageRelease:
		return "release"
	case LifecycleStageDeploy:
		return "deploy"
	case LifecycleStageOperate:
		return "operate"
	case LifecycleStageMonitor:
		return "monitor"
	case LifecycleStageOther:
		return "other"
	default:
		return "unknown"
	}
}
