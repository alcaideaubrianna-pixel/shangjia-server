#!/usr/bin/env bash
# Local-only acceptance tests. PostgreSQL fixtures use disposable schemas or
# temporary tables at 127.0.0.1:35432, database postgres, current OS user.
# Keep SQLite and PostgreSQL in separate processes: frame-level DB instances
# retain driver configuration even after a test restores gdb's config group.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
unset YOUBAN_RELAY_TEST_PG
export YOUBAN_LOCAL_COLLECT_PG=0

printf 'Testing Bot account interactions with local SQLite...\n'
go test ./addons/youban_bot/logic/sys \
  -run 'TestAccountAuthorization|TestAccountRenewal|TestAccountReport|TestAccountCreation|TestAccountManagement|TestOwnPasswordReset|TestManagedPasswordReset|TestManagedAccountDeletion' \
  -count=1 -timeout=90s

printf 'Testing report rendering, settings and payment primitives...\n'
go test ./addons/youban_publish/library/dailyreport ./addons/youban_publish/install ./addons/youban_bot/install ./internal/library/payment/epay \
  -count=1 -timeout=90s
go test ./addons/youban_publish/logic/sys \
  -run 'TestAccountSettingPatch|TestAccountNotificationCannotDisableMembershipReminder|TestDailyReportSevenCompletedDays' \
  -count=1 -timeout=90s

export YOUBAN_LOCAL_COLLECT_PG=1
printf 'Testing authorization with PostgreSQL concurrency and real account services...\n'
go test -race ./addons/youban_bot/logic/sys \
  -run '^TestAccountAuthorizationConcurrencyLocalPostgres$' -count=3 -timeout=90s
go test -race -tags youban_account_integration ./addons/youban_bot/logic/sys \
  -run '^TestAccountAuthorizationRealServicesLocalPostgres$' -count=1 -timeout=90s

printf 'Testing PostgreSQL reports, notices, deletion, renewal and concurrent workers...\n'
go test -race ./addons/youban_publish/logic/sys \
  -run 'TestAccountNotificationPreferenceLocalPostgres|TestAccountChannelAccessPauseLocalPostgres|TestPublishFailureNotificationLocalPostgres|TestAccountDeletionNoticeLocalPostgres|TestDailyReport.*LocalPostgres|TestBuildDailyReportLocalPostgres|TestTenantVipRenewalDurableLocalPostgres' \
  -count=1 -timeout=120s
go test ./addons/youban_publish/install \
  -run '^TestDailyReportCronInstallationLocalPostgres$' -count=1 -timeout=45s

printf 'Local account-management acceptance tests passed. No deployment performed.\n'
