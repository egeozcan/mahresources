package application_context

import (
	"errors"

	"gorm.io/gorm"

	"mahresources/contracts"
	"mahresources/models"
)

// storeClusterMembers is one page of a Cluster's members after a cursor, for a card
// the review page cut short (design D8). The Cluster is looked up through the same
// withheld filter the page uses, so a Cluster the principal may not fully see is an
// unknown one; the Resources behind the members are the narrow, scoped projection.
func (ctx *MahresourcesContext) storeClusterMembers(r *models.ResourceReduction, key string, after reductionMemberCursor, expectWinner *uint) (*contracts.ReductionMembersPage, error) {
	store := ctx.reductionStore()
	c, err := store.loadCluster(r, reductionGenerationOf(r), key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrReductionClusterNotFound
	}
	if err != nil {
		return nil, err
	}
	if expectWinner != nil && *expectWinner != c.Row.WinnerID {
		return nil, ErrReductionConflict
	}
	rows, err := store.memberPage(c.Row.ID, after, reductionMemberCap+1)
	if err != nil {
		return nil, err
	}
	page := &contracts.ReductionMembersPage{}
	if len(rows) > reductionMemberCap {
		rows = rows[:reductionMemberCap]
		last := rows[len(rows)-1]
		page.After = reductionMemberCursor{Position: last.Position, ID: last.ID}.String()
	}
	ids := make([]uint, 0, len(rows))
	for _, m := range rows {
		ids = append(ids, m.ResourceID)
	}
	resources, err := store.loadNarrowResources(ids)
	if err != nil {
		return nil, err
	}
	associations, err := store.associationKinds(resourceIDsOf(resources))
	if err != nil {
		return nil, err
	}
	for i := range rows {
		m := &rows[i]
		member := memberOfRow(m)
		page.Members = append(page.Members, contracts.ReductionMemberView{
			ReductionMember: member,
			Resource:        resources[m.ResourceID],
			IsWinner:        m.ResourceID == c.Row.WinnerID,
			IsLoser:         member.IsLoser(c.Row.WinnerID),
			Changed:         m.Changed,
			DistanceLabel:   reductionDistanceLabel(member.Distance),
			TagCount:        associations[m.ResourceID].Tags,
			NoteCount:       associations[m.ResourceID].Notes,
			GroupCount:      associations[m.ResourceID].Groups,
		})
	}
	return page, nil
}
