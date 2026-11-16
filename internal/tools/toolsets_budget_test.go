package tools_test

// Tool list budgets in bytes, see TestToolListBudget. Measured when the
// budgets were set: default 91,978, compact 62,029, core 39,393, core with
// compact 23,315. The snippet/fields parameters add 1,992 bytes to core and
// 1,342 to core compact (31 tools).
const (
	budgetDefault     = 95000
	budgetCompact     = 64000
	budgetCore        = 42000
	budgetCoreCompact = 25000
)
