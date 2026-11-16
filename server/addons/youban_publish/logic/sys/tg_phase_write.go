package sys

// telegramPhaseWriteSources prevents ordinary phase updates from rolling back
// confirmed remote effects. Intentional retry/reset uses typed JobTransition.
// The legacy relay pre-confirmation retry remains supported.
func telegramPhaseWriteSources(target SendPhase) []SendPhase {
	all := []SendPhase{SendPhaseEmpty, SendPhaseCleanupProcessing, SendPhaseCleanupConfirmed,
		SendPhaseDisplaySending, SendPhaseDisplayConfirmed, SendPhaseVerifySending,
		SendPhaseVerifyConfirmed, SendPhaseCompletedNoVerify,
		SendPhaseRelayPreparing, SendPhaseRelayReady, SendPhasePackageSending, SendPhasePackageConfirmed}
	var sources []SendPhase
	for _, source := range all {
		if source == target {
			sources = append(sources, source)
			continue
		}
		switch source {
		case SendPhaseVerifyConfirmed, SendPhaseCompletedNoVerify:
			continue
		case SendPhaseVerifySending:
			if target != SendPhaseVerifyConfirmed {
				continue
			}
		case SendPhaseDisplayConfirmed, SendPhasePackageConfirmed:
			if target != SendPhaseVerifySending && target != SendPhaseVerifyConfirmed && target != SendPhaseCompletedNoVerify {
				continue
			}
		}
		sources = append(sources, source)
	}
	return sources
}
