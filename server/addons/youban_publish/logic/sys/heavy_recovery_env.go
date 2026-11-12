package sys

import "os"

// heavyRecoveryDisabled 读取线上临时止血开关，只暂停低优先级维护任务。
func heavyRecoveryDisabled(name string) bool { return os.Getenv(name) == "1" }
