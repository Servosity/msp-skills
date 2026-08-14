// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (novel file, not generated).
//
// Shared SQL for the novel ticket commands, lifted out of buildTriageQuery so
// the family fixed in #209 stays fixed in one place. See issue #211.
//
// Two independent defects, both of which have to be handled together:
//
//  1. tickets.agent_name and tickets.datecreated are generated columns HaloPSA
//     never populates. The ticket payload carries agent_id and dateoccurred
//     instead, so both columns are blank on every synced row: a command reading
//     agent_name labels every ticket "(unassigned)" or "?", and one reading
//     datecreated reports every age as 0.
//
//  2. The grouping key must not be named "who". The generated tickets table has
//     a real who TEXT column that is NULL on every row, and SQLite resolves
//     GROUP BY names against source columns before output aliases, so
//     "... AS who ... GROUP BY who" groups by that NULL column and collapses
//     every agent into a single row no matter what the SELECT computed.
//
// Defect 2 hides defect 1: resolving the name per row still yields one bucket
// while the grouping key collides, which is why both fixes ship together.
//
// Commands reading FROM actions rather than FROM tickets are deliberately left
// alone. The actions table has no who column, so their "GROUP BY who" binds the
// output alias correctly, and they already attribute to real agents.

package cli

// ticketAgentScopedCTE opens a `scoped` CTE exposing every tickets column plus
// two resolved ones:
//
//	agent_label - the agent's name, resolved from the synced agent records via
//	              agent_id. A populated agent_name still wins, so a future Halo
//	              version (or an includeagent sync) needs no change here.
//	created_at  - the ticket's creation timestamp, falling through to
//	              dateoccurred when datecreated is blank.
//
// Neither name exists as a real tickets column, so neither can collide the way
// "who" does. Callers append their own SELECT ... FROM scoped and must group on
// agent_label rather than on an output alias.
const ticketAgentScopedCTE = `WITH scoped AS (
                SELECT t.*,
                    COALESCE(NULLIF(t.agent_name, ''), a.agent_label, '(unassigned)') AS agent_label,
                    COALESCE(NULLIF(t.datecreated, ''), json_extract(t.data, '$.dateoccurred')) AS created_at
                FROM tickets t
                LEFT JOIN (
                    SELECT CAST(id AS INTEGER) AS agent_ref_id,
                           json_extract(data, '$.name') AS agent_label
                    FROM resources
                    WHERE resource_type = 'agent'
                ) a ON a.agent_ref_id = t.agent_id
            )`
