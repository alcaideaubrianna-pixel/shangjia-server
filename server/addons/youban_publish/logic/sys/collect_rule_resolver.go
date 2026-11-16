package sys

import "github.com/gogf/gf/v2/database/gdb"

const collectRuleSourceIDField = "_collect_rule_source_id"

// collectRuleBindingEnabled keeps the collection policy default-on: only the
// explicit disabled value stops a rule. Empty and legacy zero values remain
// usable so old bindings continue to dispatch until an operator disables them.
func collectRuleBindingEnabled(status int) bool {
	return status != 2
}

func collectRuleSourceID(event gdb.Record, rule gdb.Record) int64 {
	if !rule.IsEmpty() && rule[collectRuleSourceIDField].Int64() > 0 {
		return rule[collectRuleSourceIDField].Int64()
	}
	if event.IsEmpty() {
		return 0
	}
	return event["source_id"].Int64()
}
