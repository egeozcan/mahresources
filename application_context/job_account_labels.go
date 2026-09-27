package application_context

import (
	"fmt"
	"slices"

	"mahresources/jobs"
	"mahresources/models"
)

// This file names the accounts a Job records as its owner and actor, so the Job
// Center can show a person rather than a user number.
//
// What a viewer may be told follows what the viewer may see. An administrator
// sees every account's Jobs and administers the accounts themselves, so every
// name is theirs to read. Anyone else sees only the Jobs they own; the one name
// they are ever told is their own, and an actor who is somebody else stays a
// number, as it was.

// JobAccountLabel is how one account is named in the Job Center: its display
// name when it has one, with the username beside it, because two accounts can
// share a display name and only the username is unique.
func JobAccountLabel(user models.User) string {
	if user.DisplayName != "" && user.DisplayName != user.Username {
		return fmt.Sprintf("%s (%s)", user.DisplayName, user.Username)
	}
	return user.Username
}

// JobAccountLabels answers the labels of the accounts one set of Jobs names, in
// one read however many Jobs name them. An id with no account behind it, and one
// this viewer may not be told, is left out.
func (ctx *MahresourcesContext) JobAccountLabels(ids []uint) (map[uint]string, error) {
	principal := ctx.Principal()
	wanted := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 || slices.Contains(wanted, id) {
			continue
		}
		if !principal.IsAdmin() && id != principal.UserID {
			continue
		}
		wanted = append(wanted, id)
	}
	labels := make(map[uint]string, len(wanted))
	if len(wanted) == 0 {
		return labels, nil
	}
	var users []models.User
	if err := ctx.db.Select("id", "username", "display_name").Where("id IN ?", wanted).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("read job account names: %w", err)
	}
	for _, user := range users {
		labels[user.ID] = JobAccountLabel(user)
	}
	return labels, nil
}

// JobAccountOption is one account an administrator can filter the Job Center by.
type JobAccountOption struct {
	ID    uint
	Label string
}

// JobAccountOptions lists the accounts an administrator can filter Jobs by, in
// username order. Anyone else is offered none: they see only their own Jobs, so
// an owner filter could only ever narrow to nothing.
func (ctx *MahresourcesContext) JobAccountOptions() ([]JobAccountOption, error) {
	if !ctx.Principal().IsAdmin() {
		return nil, nil
	}
	var users []models.User
	if err := ctx.db.Select("id", "username", "display_name").Order("username asc").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("read job accounts: %w", err)
	}
	options := make([]JobAccountOption, 0, len(users))
	for _, user := range users {
		options = append(options, JobAccountOption{ID: user.ID, Label: JobAccountLabel(user)})
	}
	return options, nil
}

// UnfinishedJobCounts counts, per account, the Jobs that have not reached an
// outcome and act as that account: what deleting the account would leave
// without anybody to run as. It is one grouped read over the nonterminal
// Jobs, whose state is indexed, however many accounts the page lists.
//
// The account a Job acts as is the one dispatch runs it as
// (jobs.ExecutionAccountSQL), so a row written before its class was recorded is
// read exactly as dispatch reads it, deletion marks included.
func (ctx *MahresourcesContext) UnfinishedJobCounts() (map[uint]int64, error) {
	var rows []struct {
		AccountID uint
		Count     int64
	}
	account, accountArgs := jobs.ExecutionAccountSQL()
	err := ctx.db.Table("(?) AS unfinished",
		ctx.db.Model(&models.Job{}).
			Select(account+" AS account_id", accountArgs...).
			Where("state NOT IN ?", []jobs.State{jobs.StateSucceeded, jobs.StateFailed, jobs.StateCancelled, jobs.StateInterrupted})).
		Select("account_id, COUNT(*) AS count").
		Where("account_id IS NOT NULL").
		Group("account_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count unfinished jobs by account: %w", err)
	}
	counts := make(map[uint]int64, len(rows))
	for _, row := range rows {
		counts[row.AccountID] = row.Count
	}
	return counts, nil
}
