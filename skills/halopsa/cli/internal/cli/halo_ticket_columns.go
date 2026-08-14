package cli

import "strings"

// HaloPSA populates neither tickets.agent_name nor tickets.datecreated. Both are
// generated columns the press derived from the OpenAPI schema, but the live
// ticket payload carries agent_id and dateoccurred instead, so on a real sync
// both columns are blank on every row (all 1254 on the instance in #203).
//
// Every local view that read them was therefore wrong in the same two ways:
// agents came out as '(unassigned)', and any age computed from datecreated came
// out as 0. The helpers below are the corrected expressions, written as drop-in
// SQL fragments so a query only has to swap an expression rather than
// restructure its FROM clause.
//
// Both are conservative: a populated agent_name or datecreated still wins, so a
// future Halo version, or a sync that includes the agent expansion, needs no
// further change here.

// haloAgentLabelExpr returns a SQL expression resolving a ticket's agent display
// name. alias is the tickets table alias ("" when the query selects from tickets
// unqualified); unassigned is the label for a ticket with no resolvable agent.
//
// The agent name is looked up from the synced agent records with a correlated
// subquery rather than a join, so it can be dropped into an existing SELECT list
// without touching the rest of the statement.
// ORDER BY r.id LIMIT 1 keeps the subquery deterministic: resources.id is TEXT,
// so ids that differ as text but not as integers ("11" and "011") both match the
// CAST comparison, and a bare LIMIT 1 would let SQLite return either name. The
// CAST is kept rather than comparing as text because ids arrive from JSON and
// their text formatting is not guaranteed stable.
//
// The inner NULLIF matters too: an agent record with a blank name would
// otherwise resolve to an empty label rather than falling through to the
// caller's unassigned text, printing a nameless row.
func haloAgentLabelExpr(alias, unassigned string) string {
	p := qualify(alias)
	return "COALESCE(NULLIF(" + p + "agent_name, ''), " +
		"(SELECT NULLIF(json_extract(r.data, '$.name'), '') FROM resources r " +
		"WHERE r.resource_type = 'agent' AND CAST(r.id AS INTEGER) = " + p + "agent_id " +
		"ORDER BY r.id LIMIT 1), " + sqlStringLiteral(unassigned) + ")"
}

// sqlStringLiteral quotes a value for inline use in generated SQL. Every current
// caller passes a fixed label, so nothing user-controlled reaches it today; this
// exists so that stops being a thing anyone has to remember.
func sqlStringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// haloTicketCreatedExpr returns a SQL expression for a ticket's creation
// timestamp, falling through to Halo's dateoccurred when the generated
// datecreated column is blank.
func haloTicketCreatedExpr(alias string) string {
	p := qualify(alias)
	return "COALESCE(NULLIF(" + p + "datecreated, ''), json_extract(" + p + "data, '$.dateoccurred'))"
}

// haloTicketActivityExpr returns a SQL expression for when a ticket was last
// touched: its most recent action, else its creation timestamp.
func haloTicketActivityExpr(alias string) string {
	p := qualify(alias)
	return "COALESCE(NULLIF(json_extract(" + p + "data, '$.lastactiondate'), ''), " + haloTicketCreatedExpr(alias) + ")"
}

func qualify(alias string) string {
	alias = strings.TrimSuffix(strings.TrimSpace(alias), ".")
	if alias == "" {
		return ""
	}
	return alias + "."
}
