package application_context

import (
	"encoding/json"
	"log"
	"strings"

	"mahresources/lib"
	"mahresources/models"
)

// syncMentionsForNote parses @-mentions from the note's description and block content,
// then adds any referenced entities as relations (additive only, safe for form saves).
func (ctx *MahresourcesContext) syncMentionsForNote(note *models.Note) {
	ctx.syncMentionsForNoteReplace(note, false)
}

// syncMentionsForNoteReplace parses @-mentions and syncs relations.
// When replace is true, it replaces all tag/group/resource associations with exactly
// the mentioned set (used by block saves to avoid stale relation accumulation).
// When replace is false, it only appends (safe for form saves that manage their own relations).
func (ctx *MahresourcesContext) syncMentionsForNoteReplace(note *models.Note, replace bool) {
	// Gather all text: description + text blocks
	var parts []string
	parts = append(parts, note.Description)

	var blocks []models.NoteBlock
	if err := ctx.db.Where("note_id = ? AND type = ?", note.ID, "text").Find(&blocks).Error; err == nil {
		for _, block := range blocks {
			var content struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(block.Content, &content) == nil {
				parts = append(parts, content.Text)
			}
		}
	}

	text := strings.Join(parts, "\n")

	mentions := lib.ParseMentions(text)
	grouped := lib.GroupMentionsByType(mentions)

	if replace {
		// Replace mode: set associations to exactly the mentioned set
		tags := BuildAssociationSlice(grouped["tag"], TagFromID)
		if err := ctx.db.Model(note).Association("Tags").Replace(&tags); err != nil {
			log.Printf("mention sync: failed to replace tags on note %d: %v", note.ID, err)
		}

		groups := BuildAssociationSlice(grouped["group"], GroupFromID)
		if err := ctx.db.Model(note).Association("Groups").Replace(&groups); err != nil {
			log.Printf("mention sync: failed to replace groups on note %d: %v", note.ID, err)
		}

		resources := BuildAssociationSlice(grouped["resource"], ResourceFromID)
		if err := ctx.db.Model(note).Association("Resources").Replace(&resources); err != nil {
			log.Printf("mention sync: failed to replace resources on note %d: %v", note.ID, err)
		}
	} else {
		// Append mode: only add, never remove (safe alongside form-set relations)
		if ids, ok := grouped["tag"]; ok {
			if err := ctx.AddTagsToNote(note.ID, ids); err != nil {
				log.Printf("mention sync: failed to add tags to note %d: %v", note.ID, err)
			}
		}
		if ids, ok := grouped["group"]; ok {
			if err := ctx.AddGroupsToNote(note.ID, ids); err != nil {
				log.Printf("mention sync: failed to add groups to note %d: %v", note.ID, err)
			}
		}
		if ids, ok := grouped["resource"]; ok {
			if err := ctx.AddResourcesToNote(note.ID, ids); err != nil {
				log.Printf("mention sync: failed to add resources to note %d: %v", note.ID, err)
			}
		}
	}
}

// syncMentionsForGroup parses @-mentions from the group's description
// and adds any referenced entities as relations.
// Tags and RelatedGroups use Append (also managed by form).
// RelatedNotes and RelatedResources use Replace (solely mention-managed).
func (ctx *MahresourcesContext) syncMentionsForGroup(group *models.Group) {
	mentions := lib.ParseMentions(group.Description)
	grouped := lib.GroupMentionsByType(mentions)

	// Tags: Append (form also manages these)
	if ids, ok := grouped["tag"]; ok {
		tags := BuildAssociationSlice(ids, TagFromID)
		if err := ctx.db.Model(group).Association("Tags").Append(&tags); err != nil {
			log.Printf("mention sync: failed to add tags to group %d: %v", group.ID, err)
		}
	}

	// RelatedNotes: Replace (solely mention-managed for groups)
	notes := BuildAssociationSlice(grouped["note"], NoteFromID)
	if err := ctx.db.Model(group).Association("RelatedNotes").Replace(&notes); err != nil {
		log.Printf("mention sync: failed to replace notes on group %d: %v", group.ID, err)
	}

	// RelatedResources: Replace (solely mention-managed for groups)
	resources := BuildAssociationSlice(grouped["resource"], ResourceFromID)
	if err := ctx.db.Model(group).Association("RelatedResources").Replace(&resources); err != nil {
		log.Printf("mention sync: failed to replace resources on group %d: %v", group.ID, err)
	}

	// RelatedGroups: Append (form also manages these)
	if ids, ok := grouped["group"]; ok {
		groups := BuildAssociationSlice(ids, GroupFromID)
		if err := ctx.db.Model(group).Association("RelatedGroups").Append(&groups); err != nil {
			log.Printf("mention sync: failed to add groups to group %d: %v", group.ID, err)
		}
	}
}

// syncMentionsForResource parses @-mentions from the resource's description
// and adds any referenced entities as relations.
func (ctx *MahresourcesContext) syncMentionsForResource(resource *models.Resource) {
	mentions := lib.ParseMentions(resource.Description)
	if len(mentions) == 0 {
		return
	}

	grouped := lib.GroupMentionsByType(mentions)

	if ids, ok := grouped["tag"]; ok {
		tags := BuildAssociationSlice(ids, TagFromID)
		if err := ctx.db.Model(resource).Association("Tags").Append(&tags); err != nil {
			log.Printf("mention sync: failed to add tags to resource %d: %v", resource.ID, err)
		}
	}
	if ids, ok := grouped["note"]; ok {
		notes := BuildAssociationSlice(ids, NoteFromID)
		if err := ctx.db.Model(resource).Association("Notes").Append(&notes); err != nil {
			log.Printf("mention sync: failed to add notes to resource %d: %v", resource.ID, err)
		}
	}
	if ids, ok := grouped["group"]; ok {
		groups := BuildAssociationSlice(ids, GroupFromID)
		if err := ctx.db.Model(resource).Association("Groups").Append(&groups); err != nil {
			log.Printf("mention sync: failed to add groups to resource %d: %v", resource.ID, err)
		}
	}
}
