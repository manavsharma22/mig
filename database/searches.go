// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Contributor: Julien Vehent jvehent@mozilla.com [:ulfr]

package database /* import "mig.ninja/mig/database" */

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"mig.ninja/mig"

	_ "github.com/lib/pq"
)

// SearchParameters contains fields used to perform database searches
type SearchParameters struct {
	ActionID         string    `json:"actionid"`
	ActionName       string    `json:"actionname"`
	After            time.Time `json:"after"`
	AgentID          string    `json:"agentid"`
	AgentName        string    `json:"agentname"`
	Before           time.Time `json:"before"`
	CommandID        string    `json:"commandid"`
	FoundAnything    bool      `json:"foundanything"`
	InvestigatorID   string    `json:"investigatorid"`
	InvestigatorName string    `json:"investigatorname"`
	Limit            float64   `json:"limit"`
	Offset           float64   `json:"offset"`
	Report           string    `json:"report"`
	Status           string    `json:"status"`
	Target           string    `json:"target"`
	ThreatFamily     string    `json:"threatfamily"`
	Type             string    `json:"type"`
}

// 10 years
const defaultSearchPeriod time.Duration = 39600 * time.Hour

// NewSearchParameters initializes search parameters
func NewSearchParameters() (p SearchParameters) {
	p.Before = time.Now().Add(defaultSearchPeriod).UTC()
	p.After = time.Now().Add(-defaultSearchPeriod).UTC()
	p.AgentName = "%"
	p.AgentID = "∞"
	p.ActionName = "%"
	p.ActionID = "∞"
	p.CommandID = "∞"
	p.ThreatFamily = "%"
	p.Status = "%"
	p.Limit = 100
	p.Offset = 0
	p.InvestigatorID = "∞"
	p.InvestigatorName = "%"
	p.Type = "action"
	return
}

// String() returns a query string with the current search parameters
func (p SearchParameters) String() (query string) {
	query = fmt.Sprintf("type=%s&after=%s&before=%s", p.Type, p.After.Format(time.RFC3339), p.Before.Format(time.RFC3339))
	if p.AgentName != "%" {
		query += fmt.Sprintf("&agentname=%s", p.AgentName)
	}
	if p.AgentID != "∞" {
		query += fmt.Sprintf("&agentid=%s", p.AgentID)
	}
	if p.ActionName != "%" {
		query += fmt.Sprintf("&actionname=%s", p.ActionName)
	}
	if p.ActionID != "∞" {
		query += fmt.Sprintf("&actionid=%s", p.ActionID)
	}
	if p.CommandID != "∞" {
		query += fmt.Sprintf("&commandid=%s", p.CommandID)
	}
	if p.InvestigatorID != "∞" {
		query += fmt.Sprintf("&investigatorid=%s", p.InvestigatorID)
	}
	if p.InvestigatorName != "%" {
		query += fmt.Sprintf("&investigatorname=%s", p.InvestigatorName)
	}
	if p.ThreatFamily != "%" {
		query += fmt.Sprintf("&threatfamily=%s", p.ThreatFamily)
	}
	if p.Status != "%" {
		query += fmt.Sprintf("&status=%s", p.Status)
	}
	query += fmt.Sprintf("&limit=%.0f", p.Limit)
	if p.Offset != 0 {
		query += fmt.Sprintf("&offset=%.0f", p.Offset)
	}
	return
}

type IDs struct {
	minActionID, maxActionID, minCommandID, maxCommandID, minAgentID, maxAgentID, minInvID, maxInvID float64
}

const MAXFLOAT64 float64 = 9007199254740991 // 2^53-1

func makeIDsFromParams(p SearchParameters) (ids IDs, err error) {
	ids.minActionID = 0
	ids.maxActionID = MAXFLOAT64
	if p.ActionID != "∞" {
		ids.minActionID, err = strconv.ParseFloat(p.ActionID, 64)
		if err != nil {
			return
		}
		ids.maxActionID = ids.minActionID
	}
	ids.minCommandID = 0
	ids.maxCommandID = MAXFLOAT64
	if p.CommandID != "∞" {
		ids.minCommandID, err = strconv.ParseFloat(p.CommandID, 64)
		if err != nil {
			return
		}
		ids.maxCommandID = ids.minCommandID
	}
	ids.minAgentID = 0
	ids.maxAgentID = MAXFLOAT64
	if p.AgentID != "∞" {
		ids.minAgentID, err = strconv.ParseFloat(p.AgentID, 64)
		if err != nil {
			return
		}
		ids.maxAgentID = ids.minAgentID
	}
	ids.minInvID = 0
	ids.maxInvID = MAXFLOAT64
	if p.InvestigatorID != "∞" {
		ids.minInvID, err = strconv.ParseFloat(p.InvestigatorID, 64)
		if err != nil {
			return
		}
		ids.maxInvID = ids.minInvID
	}
	return
}

// SearchCommands returns an array of commands that match search parameters
func (db *DB) SearchCommands(p SearchParameters, doFoundAnything bool) (commands []mig.Command, err error) {
	var (
		rows *sql.Rows
	)
	ids, err := makeIDsFromParams(p)
	if err != nil {
		return
	}
	query := `SELECT commands.id, commands.status, commands.results, commands.starttime, commands.finishtime,
			actions.id, actions.name, actions.target, actions.description, actions.threat,
			actions.operations, actions.validfrom, actions.expireafter, actions.pgpsignatures,
			actions.syntaxversion, agents.id, agents.name, agents.version, agents.tags, agents.environment
		FROM	commands
			INNER JOIN actions ON ( commands.actionid = actions.id)
			INNER JOIN signatures ON ( actions.id = signatures.actionid )
			INNER JOIN investigators ON ( signatures.investigatorid = investigators.id )
			INNER JOIN agents ON ( commands.agentid = agents.id )
		WHERE `
	vals := []interface{}{}
	valctr := 0
	if p.Before.Before(time.Now().Add(defaultSearchPeriod - time.Hour)) {
		query += fmt.Sprintf(`commands.starttime <= $%d `, valctr+1)
		vals = append(vals, p.Before)
		valctr += 1
	}
	if p.After.After(time.Now().Add(-(defaultSearchPeriod - time.Hour))) {
		if valctr > 0 {
			query += " AND "
		}
		query += fmt.Sprintf(`commands.starttime >= $%d `, valctr+1)
		vals = append(vals, p.After)
		valctr += 1
	}
	if p.CommandID != "∞" {
		if valctr > 0 {
			query += " AND "
		}
		query += fmt.Sprintf(`commands.id >= $%d AND commands.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minCommandID, ids.maxCommandID)
		valctr += 2
	}
	if p.Status != "%" {
		if valctr > 0 {
			query += " AND "
		}
		query += fmt.Sprintf(`commands.status ILIKE $%d`, valctr+1)
		vals = append(vals, p.Status)
		valctr += 1
	}
	if p.ActionID != "∞" {
		if valctr > 0 {
			query += " AND "
		}
		query += fmt.Sprintf(`actions.id >= $%d AND actions.id <= $%d`, valctr+1, valctr+2)
		vals = append(vals, ids.minActionID, ids.maxActionID)
		valctr += 2
	}
	if p.ActionName != "%" {
		if valctr > 0 {
			query += " AND "
		}
		query += fmt.Sprintf(`actions.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.ActionName)
		valctr += 1
	}
	if p.InvestigatorID != "∞" {
		if valctr > 0 {
			query += " AND "
		}
		query += fmt.Sprintf(`investigators.id >= $%d AND investigators.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minInvID, ids.maxInvID)
		valctr += 2
	}
	if p.InvestigatorName != "%" {
		if valctr > 0 {
			query += " AND "
		}
		query += fmt.Sprintf(`investigators.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.InvestigatorName)
		valctr += 1
	}
	if p.AgentID != "∞" {
		if valctr > 0 {
			query += " AND "
		}
		query += fmt.Sprintf(`agents.id >= $%d AND agents.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minAgentID, ids.maxAgentID)
		valctr += 2
	}
	if p.AgentName != "%" {
		if valctr > 0 {
			query += " AND "
		}
		query += fmt.Sprintf(`agents.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.AgentName)
		valctr += 1
	}
	if doFoundAnything {
		if valctr > 0 {
			query += " AND "
		}
		query += fmt.Sprintf(`commands.status = $%d
			AND commands.id IN (	SELECT commands.id FROM commands, actions, json_array_elements(commands.results) as r
						WHERE commands.actionid=actions.id
						AND actions.id >= $%d AND actions.id <= $%d
						AND r#>>'{foundanything}' = $%d) `,
			valctr+1, valctr+2, valctr+3, valctr+4)
		vals = append(vals, mig.StatusSuccess, ids.minActionID, ids.maxActionID, p.FoundAnything)
		valctr += 4
	}
	if p.ThreatFamily != "%" {
		if valctr > 0 {
			query += " AND "
		}
		query += fmt.Sprintf(`actions.threat#>>'{family}' ILIKE $%d `, valctr+1)
		vals = append(vals, p.ThreatFamily)
		valctr += 1
	}
	query += fmt.Sprintf(` GROUP BY commands.id, actions.id, agents.id
		ORDER BY commands.starttime DESC LIMIT $%d OFFSET $%d;`, valctr+1, valctr+2)
	vals = append(vals, uint64(p.Limit), uint64(p.Offset))

	stmt, err := db.c.Prepare(query)
	if stmt != nil {
		defer stmt.Close()
	}
	if err != nil {
		err = fmt.Errorf("Error while preparing search statement: '%v' in '%s'", err, query)
		return
	}
	rows, err = stmt.Query(vals...)
	if rows != nil {
		defer rows.Close()
	}
	if err != nil {
		err = fmt.Errorf("Error while finding commands: '%v'", err)
		return
	}
	for rows.Next() {
		var jRes, jDesc, jThreat, jOps, jSig, jAgtTags, jAgtEnv []byte
		var cmd mig.Command
		err = rows.Scan(&cmd.ID, &cmd.Status, &jRes, &cmd.StartTime, &cmd.FinishTime,
			&cmd.Action.ID, &cmd.Action.Name, &cmd.Action.Target, &jDesc, &jThreat, &jOps,
			&cmd.Action.ValidFrom, &cmd.Action.ExpireAfter, &jSig, &cmd.Action.SyntaxVersion,
			&cmd.Agent.ID, &cmd.Agent.Name, &cmd.Agent.Version, &jAgtTags, &jAgtEnv)
		if err != nil {
			err = fmt.Errorf("Failed to retrieve command: '%v'", err)
			return
		}
		err = json.Unmarshal(jThreat, &cmd.Action.Threat)
		if err != nil {
			err = fmt.Errorf("Failed to unmarshal action threat: '%v'", err)
			return
		}
		err = json.Unmarshal(jRes, &cmd.Results)
		if err != nil {
			err = fmt.Errorf("Failed to unmarshal command results: '%v'", err)
			return
		}
		err = json.Unmarshal(jDesc, &cmd.Action.Description)
		if err != nil {
			err = fmt.Errorf("Failed to unmarshal action description: '%v'", err)
			return
		}
		err = json.Unmarshal(jOps, &cmd.Action.Operations)
		if err != nil {
			err = fmt.Errorf("Failed to unmarshal action operations: '%v'", err)
			return
		}
		err = json.Unmarshal(jSig, &cmd.Action.PGPSignatures)
		if err != nil {
			err = fmt.Errorf("Failed to unmarshal action signatures: '%v'", err)
			return
		}
		err = json.Unmarshal(jAgtTags, &cmd.Agent.Tags)
		if err != nil {
			err = fmt.Errorf("Failed to unmarshal agent tags: '%v'", err)
			return
		}
		err = json.Unmarshal(jAgtEnv, &cmd.Agent.Env)
		if err != nil {
			err = fmt.Errorf("Failed to unmarshal agent environment: '%v'", err)
			return
		}
		cmd.Action.Counters, err = db.GetActionCounters(cmd.Action.ID)
		if err != nil {
			err = fmt.Errorf("Failed to retrieve action counters: '%v'", err)
			return
		}
		cmd.Action.Investigators, err = db.InvestigatorByActionID(cmd.Action.ID)
		if err != nil {
			err = fmt.Errorf("Failed to retrieve action investigators: '%v'", err)
			return
		}
		commands = append(commands, cmd)
	}
	if err := rows.Err(); err != nil {
		err = fmt.Errorf("Failed to complete database query: '%v'", err)
	}
	return
}

// SearchActions returns an array of actions that match search parameters
func (db *DB) SearchActions(p SearchParameters) (actions []mig.Action, err error) {
	var (
		rows                                     *sql.Rows
		joinAgent, joinInvestigator, joinCommand bool = false, false, false
	)
	ids, err := makeIDsFromParams(p)
	if err != nil {
		return
	}
	columns := `actions.id, actions.name, actions.target,  actions.description, actions.threat, actions.operations,
		actions.validfrom, actions.expireafter, actions.starttime, actions.finishtime, actions.lastupdatetime,
		actions.status, actions.pgpsignatures, actions.syntaxversion `
	join := ""
	where := ""
	vals := []interface{}{}
	valctr := 0
	if p.Before.Before(time.Now().Add(defaultSearchPeriod - time.Hour)) {
		where += fmt.Sprintf(`actions.expireafter <= $%d `, valctr+1)
		vals = append(vals, p.Before)
		valctr += 1
	}
	if p.After.After(time.Now().Add(-(defaultSearchPeriod - time.Hour))) {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`actions.validfrom >= $%d `, valctr+1)
		vals = append(vals, p.After)
		valctr += 1
	}
	if p.Status != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`action.status ILIKE $%d`, valctr+1)
		vals = append(vals, p.Status)
		valctr += 1
	}
	if p.ActionID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`actions.id >= $%d AND actions.id <= $%d`, valctr+1, valctr+2)
		vals = append(vals, ids.minActionID, ids.maxActionID)
		valctr += 2
	}
	if p.ActionName != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`actions.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.ActionName)
		valctr += 1
	}
	if p.InvestigatorID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`investigators.id >= $%d AND investigators.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minInvID, ids.maxInvID)
		valctr += 2
		joinInvestigator = true
	}
	if p.InvestigatorName != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`investigators.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.InvestigatorName)
		valctr += 1
		joinInvestigator = true
	}
	if p.AgentID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`agents.id >= $%d AND agents.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minAgentID, ids.maxAgentID)
		valctr += 2
		joinAgent = true
		joinCommand = true
	}
	if p.AgentName != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`agents.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.AgentName)
		valctr += 1
		joinAgent = true
		joinCommand = true
	}
	if p.CommandID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`commands.id >= $%d AND commands.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minCommandID, ids.maxCommandID)
		valctr += 2
		joinCommand = true
	}
	if joinCommand {
		join += "INNER JOIN commands ON ( commands.actionid = actions.id) "
	}
	if joinAgent {
		join += " INNER JOIN agents ON ( commands.agentid = agents.id ) "
	}
	if joinInvestigator {
		join += ` INNER JOIN signatures ON ( actions.id = signatures.actionid )
			INNER JOIN investigators ON ( signatures.investigatorid = investigators.id ) `
	}
	if p.ThreatFamily != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`actions.threat#>>'{family}' ILIKE $%d `, valctr+1)
		vals = append(vals, p.ThreatFamily)
		valctr += 1
	}
	query := fmt.Sprintf(`SELECT %s FROM actions %s WHERE %s GROUP BY actions.id
		ORDER BY actions.validfrom DESC LIMIT $%d OFFSET $%d;`,
		columns, join, where, valctr+1, valctr+2)
	vals = append(vals, uint64(p.Limit), uint64(p.Offset))

	stmt, err := db.c.Prepare(query)
	if stmt != nil {
		defer stmt.Close()
	}
	if err != nil {
		err = fmt.Errorf("Error while preparing search statement: '%v' in '%s'", err, query)
		return
	}
	rows, err = stmt.Query(vals...)
	if rows != nil {
		defer rows.Close()
	}
	if err != nil {
		err = fmt.Errorf("Error while finding actions: '%v'", err)
		return
	}
	for rows.Next() {
		var jDesc, jThreat, jOps, jSig []byte
		var a mig.Action
		err = rows.Scan(&a.ID, &a.Name, &a.Target,
			&jDesc, &jThreat, &jOps, &a.ValidFrom, &a.ExpireAfter,
			&a.StartTime, &a.FinishTime, &a.LastUpdateTime, &a.Status,
			&jSig, &a.SyntaxVersion)
		if err != nil {
			err = fmt.Errorf("Error while retrieving action: '%v'", err)
			return
		}
		err = json.Unmarshal(jThreat, &a.Threat)
		if err != nil {
			err = fmt.Errorf("Failed to unmarshal action threat: '%v'", err)
			return
		}
		err = json.Unmarshal(jDesc, &a.Description)
		if err != nil {
			err = fmt.Errorf("Failed to unmarshal action description: '%v'", err)
			return
		}
		err = json.Unmarshal(jOps, &a.Operations)
		if err != nil {
			err = fmt.Errorf("Failed to unmarshal action operations: '%v'", err)
			return
		}
		err = json.Unmarshal(jSig, &a.PGPSignatures)
		if err != nil {
			err = fmt.Errorf("Failed to unmarshal action signatures: '%v'", err)
			return
		}
		a.Counters, err = db.GetActionCounters(a.ID)
		if err != nil {
			err = fmt.Errorf("Failed to retrieve action counters: '%v'", err)
			return
		}
		a.Investigators, err = db.InvestigatorByActionID(a.ID)
		if err != nil {
			err = fmt.Errorf("Failed to retrieve action investigators: '%v'", err)
			return
		}
		actions = append(actions, a)
	}
	if err := rows.Err(); err != nil {
		err = fmt.Errorf("Failed to complete database query: '%v'", err)
	}
	return
}

// SearchAgents returns an array of agents that match search parameters
func (db *DB) SearchAgents(p SearchParameters) (agents []mig.Agent, err error) {
	var (
		rows                                      *sql.Rows
		joinAction, joinInvestigator, joinCommand bool = false, false, false
	)
	ids, err := makeIDsFromParams(p)
	if err != nil {
		return
	}
	columns := `agents.id, agents.name, agents.queueloc, agents.mode,
		agents.version, agents.pid, agents.starttime, agents.destructiontime,
		agents.heartbeattime, agents.status`
	join := ""
	where := ""
	vals := []interface{}{}
	valctr := 0
	if p.Before.Before(time.Now().Add(defaultSearchPeriod - time.Hour)) {
		where += fmt.Sprintf(`agents.heartbeattime <= $%d `, valctr+1)
		vals = append(vals, p.Before)
		valctr += 1
	}
	if p.After.After(time.Now().Add(-(defaultSearchPeriod - time.Hour))) {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`agents.heartbeattime >= $%d `, valctr+1)
		vals = append(vals, p.After)
		valctr += 1
	}
	if p.AgentID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`agents.id >= $%d AND agents.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minAgentID, ids.maxAgentID)
		valctr += 2
	}
	if p.AgentName != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`agents.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.AgentName)
		valctr += 1
	}
	if p.Status != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`agents.status ILIKE $%d`, valctr+1)
		vals = append(vals, p.Status)
		valctr += 1
	}
	if p.ActionID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`actions.id >= $%d AND actions.id <= $%d`, valctr+1, valctr+2)
		vals = append(vals, ids.minActionID, ids.maxActionID)
		valctr += 2
		joinAction = true
		joinCommand = true
	}
	if p.ActionName != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`actions.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.ActionName)
		valctr += 1
		joinAction = true
		joinCommand = true
	}
	if p.ThreatFamily != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`actions.threat#>>'{family}' ILIKE $%d `, valctr+1)
		vals = append(vals, p.ThreatFamily)
		valctr += 1
		joinAction = true
		joinCommand = true
	}
	if p.InvestigatorID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`investigators.id >= $%d AND investigators.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minInvID, ids.maxInvID)
		valctr += 2
		joinInvestigator = true
		joinCommand = true
		joinAction = true
	}
	if p.InvestigatorName != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`investigators.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.InvestigatorName)
		valctr += 1
		joinInvestigator = true
		joinCommand = true
		joinAction = true
	}
	if p.CommandID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`commands.id >= $%d AND commands.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minCommandID, ids.maxCommandID)
		valctr += 2
		joinCommand = true
	}
	if joinCommand {
		join += "INNER JOIN commands ON ( commands.agentid = agents.id) "
	}
	if joinAction {
		join += " INNER JOIN actions ON ( commands.actionid = actions.id ) "
	}
	if joinInvestigator {
		join += ` INNER JOIN signatures ON ( actions.id = signatures.actionid )
			INNER JOIN investigators ON ( signatures.investigatorid = investigators.id ) `
	}
	query := fmt.Sprintf(`SELECT %s FROM agents %s WHERE %s GROUP BY agents.id
		ORDER BY agents.heartbeattime DESC LIMIT $%d OFFSET $%d;`,
		columns, join, where, valctr+1, valctr+2)
	vals = append(vals, uint64(p.Limit), uint64(p.Offset))

	stmt, err := db.c.Prepare(query)
	if stmt != nil {
		defer stmt.Close()
	}
	if err != nil {
		err = fmt.Errorf("Error while preparing search statement: '%v' in '%s'", err, query)
		return
	}
	rows, err = stmt.Query(vals...)
	if rows != nil {
		defer rows.Close()
	}
	if err != nil {
		err = fmt.Errorf("Error while finding agents: '%v'", err)
		return
	}
	for rows.Next() {
		var agent mig.Agent
		err = rows.Scan(&agent.ID, &agent.Name, &agent.QueueLoc, &agent.Mode, &agent.Version,
			&agent.PID, &agent.StartTime, &agent.DestructionTime, &agent.HeartBeatTS,
			&agent.Status)
		if err != nil {
			err = fmt.Errorf("Failed to retrieve agent data: '%v'", err)
			return
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		err = fmt.Errorf("Failed to complete database query: '%v'", err)
	}

	return
}

// SearchInvestigators returns an array of investigators that match search parameters
func (db *DB) SearchInvestigators(p SearchParameters) (investigators []mig.Investigator, err error) {
	var (
		rows                               *sql.Rows
		joinAction, joinAgent, joinCommand bool = false, false, false
	)
	ids, err := makeIDsFromParams(p)
	if err != nil {
		return
	}
	columns := `investigators.id, investigators.name, investigators.pgpfingerprint,
		investigators.status, investigators.createdat, investigators.lastmodified`
	join := ""
	where := ""
	vals := []interface{}{}
	valctr := 0
	if p.Before.Before(time.Now().Add(defaultSearchPeriod - time.Hour)) {
		where += fmt.Sprintf(`investigators.lastmodified <= $%d `, valctr+1)
		vals = append(vals, p.Before)
		valctr += 1
	}
	if p.After.After(time.Now().Add(-(defaultSearchPeriod - time.Hour))) {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`investigators.lastmodified >= $%d `, valctr+1)
		vals = append(vals, p.After)
		valctr += 1
	}
	if p.InvestigatorID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`investigators.id >= $%d AND investigators.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minInvID, ids.maxInvID)
		valctr += 2
	}
	if p.InvestigatorName != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`investigators.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.InvestigatorName)
		valctr += 1
	}
	if p.Status != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`investigators.status ILIKE $%d`, valctr+1)
		vals = append(vals, p.Status)
		valctr += 1
	}
	if p.ActionID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`actions.id >= $%d AND actions.id <= $%d`, valctr+1, valctr+2)
		vals = append(vals, ids.minActionID, ids.maxActionID)
		valctr += 2
		joinAction = true
	}
	if p.ActionName != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`actions.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.ActionName)
		valctr += 1
		joinAction = true
	}
	if p.ThreatFamily != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`actions.threat#>>'{family}' ILIKE $%d `, valctr+1)
		vals = append(vals, p.ThreatFamily)
		valctr += 1
		joinAction = true
	}
	if p.CommandID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`commands.id >= $%d AND commands.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minCommandID, ids.maxCommandID)
		valctr += 2
		joinCommand = true
		joinAction = true
	}
	if p.AgentID != "∞" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`agents.id >= $%d AND agents.id <= $%d`,
			valctr+1, valctr+2)
		vals = append(vals, ids.minAgentID, ids.maxAgentID)
		valctr += 2
		joinCommand = true
		joinAction = true
		joinAgent = true
	}
	if p.AgentName != "%" {
		if valctr > 0 {
			where += " AND "
		}
		where += fmt.Sprintf(`agents.name ILIKE $%d`, valctr+1)
		vals = append(vals, p.AgentName)
		valctr += 1
		joinCommand = true
		joinAction = true
		joinAgent = true
	}
	if joinAction {
		join += ` INNER JOIN signatures ON ( signatures.investigatorid = investigators.id ) 
			INNER JOIN actions ON ( actions.id = signatures.actionid ) `
	}
	if joinCommand {
		join += "INNER JOIN commands ON ( commands.actionid = actions.id) "
	}
	if joinAgent {
		join += " INNER JOIN agents ON ( commands.agentid = agents.id ) "
	}
	query := fmt.Sprintf(`SELECT %s FROM investigators %s WHERE %s GROUP BY investigators.id
		ORDER BY investigators.id ASC LIMIT $%d OFFSET $%d;`,
		columns, join, where, valctr+1, valctr+2)
	vals = append(vals, uint64(p.Limit), uint64(p.Offset))

	stmt, err := db.c.Prepare(query)
	if stmt != nil {
		defer stmt.Close()
	}
	if err != nil {
		err = fmt.Errorf("Error while preparing search statement: '%v' in '%s'", err, query)
		return
	}
	rows, err = stmt.Query(vals...)
	if rows != nil {
		defer rows.Close()
	}
	if err != nil {
		err = fmt.Errorf("Error while finding investigators: '%v'", err)
		return
	}
	for rows.Next() {
		var inv mig.Investigator
		err = rows.Scan(&inv.ID, &inv.Name, &inv.PGPFingerprint, &inv.Status, &inv.CreatedAt, &inv.LastModified)
		if err != nil {
			err = fmt.Errorf("Failed to retrieve investigator data: '%v'", err)
			return
		}
		investigators = append(investigators, inv)
	}
	if err := rows.Err(); err != nil {
		err = fmt.Errorf("Failed to complete database query: '%v'", err)
	}
	return
}