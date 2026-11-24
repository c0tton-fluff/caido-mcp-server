package tools_test

// Tool list budgets in bytes, see TestToolListBudget. Measured at v4.5.0:
// default 91,978, compact 62,029, core 39,393, core with compact 23,315.
//
// Raised since for parameters added after v4.5.0 and for caido_call_plugin.
// The WebSocket draft tools add 3,978 bytes to default, 2,397 to compact and
// 1,293 to core (both are in the websocket toolset, which core includes):
// measured with the plugin tool in the list, 102,770 / 68,998 / 45,124 /
// 26,503. The budgets leave 2 to 4% above that for the next addition.
const (
	budgetDefault     = 105500
	budgetCompact     = 71000
	budgetCore        = 46500
	budgetCoreCompact = 27000
)
