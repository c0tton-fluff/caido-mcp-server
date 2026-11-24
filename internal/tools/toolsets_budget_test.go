package tools_test

// Tool list budgets in bytes, see TestToolListBudget. Measured at v4.5.0:
// default 91,978, compact 62,029, core 39,393, core with compact 23,315.
//
// Raised once since, for parameters added after v4.5.0 (bodyEncoding on
// caido_get_request and caido_get_replay_entry, snippet and fields on
// caido_list_requests): measured 95,362 / 64,028 / 42,777 / 25,314. The
// budgets leave 2 to 4% above that for the next addition; a change that
// needs more should shorten a description before it raises a budget.
const (
	budgetDefault     = 97500
	budgetCompact     = 65500
	budgetCore        = 44500
	budgetCoreCompact = 26500
)
