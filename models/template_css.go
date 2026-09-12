package models

import "strings"

// TemplateCSS combines shared and per-slot global styles in a stable cascade order.
func (c Category) TemplateCSS() string {
	parts := []string{c.CustomCSS}
	if c.CustomHeaderCSS != "" {
		parts = append(parts, c.CustomHeaderCSS)
	}
	if c.CustomDetailFooterCSS != "" {
		parts = append(parts, c.CustomDetailFooterCSS)
	}
	if c.CustomSidebarCSS != "" {
		parts = append(parts, c.CustomSidebarCSS)
	}
	if c.CustomSummaryCSS != "" {
		parts = append(parts, c.CustomSummaryCSS)
	}
	if c.CustomAvatarCSS != "" {
		parts = append(parts, c.CustomAvatarCSS)
	}
	if c.CustomHoverCardCSS != "" {
		parts = append(parts, c.CustomHoverCardCSS)
	}
	if c.CustomOwnEntitiesCSS != "" {
		parts = append(parts, c.CustomOwnEntitiesCSS)
	}
	if c.CustomListHeaderCSS != "" {
		parts = append(parts, c.CustomListHeaderCSS)
	}
	if c.CustomListFooterCSS != "" {
		parts = append(parts, c.CustomListFooterCSS)
	}
	if c.CustomMRQLResultCSS != "" {
		parts = append(parts, c.CustomMRQLResultCSS)
	}
	return strings.Join(parts, "\n")
}
func (c ResourceCategory) TemplateCSS() string {
	parts := []string{c.CustomCSS}
	if c.CustomHeaderCSS != "" {
		parts = append(parts, c.CustomHeaderCSS)
	}
	if c.CustomDetailFooterCSS != "" {
		parts = append(parts, c.CustomDetailFooterCSS)
	}
	if c.CustomSidebarCSS != "" {
		parts = append(parts, c.CustomSidebarCSS)
	}
	if c.CustomPreviewCSS != "" {
		parts = append(parts, c.CustomPreviewCSS)
	}
	if c.CustomLightboxCSS != "" {
		parts = append(parts, c.CustomLightboxCSS)
	}
	if c.CustomSummaryCSS != "" {
		parts = append(parts, c.CustomSummaryCSS)
	}
	if c.CustomAvatarCSS != "" {
		parts = append(parts, c.CustomAvatarCSS)
	}
	if c.CustomHoverCardCSS != "" {
		parts = append(parts, c.CustomHoverCardCSS)
	}
	if c.CustomCellCSS != "" {
		parts = append(parts, c.CustomCellCSS)
	}
	if c.CustomListHeaderCSS != "" {
		parts = append(parts, c.CustomListHeaderCSS)
	}
	if c.CustomListFooterCSS != "" {
		parts = append(parts, c.CustomListFooterCSS)
	}
	if c.CustomMRQLResultCSS != "" {
		parts = append(parts, c.CustomMRQLResultCSS)
	}
	return strings.Join(parts, "\n")
}
func (c NoteType) TemplateCSS() string {
	parts := []string{c.CustomCSS}
	if c.CustomHeaderCSS != "" {
		parts = append(parts, c.CustomHeaderCSS)
	}
	if c.CustomDetailFooterCSS != "" {
		parts = append(parts, c.CustomDetailFooterCSS)
	}
	if c.CustomSidebarCSS != "" {
		parts = append(parts, c.CustomSidebarCSS)
	}
	if c.CustomSummaryCSS != "" {
		parts = append(parts, c.CustomSummaryCSS)
	}
	if c.CustomAvatarCSS != "" {
		parts = append(parts, c.CustomAvatarCSS)
	}
	if c.CustomHoverCardCSS != "" {
		parts = append(parts, c.CustomHoverCardCSS)
	}
	if c.CustomListHeaderCSS != "" {
		parts = append(parts, c.CustomListHeaderCSS)
	}
	if c.CustomListFooterCSS != "" {
		parts = append(parts, c.CustomListFooterCSS)
	}
	if c.CustomMRQLResultCSS != "" {
		parts = append(parts, c.CustomMRQLResultCSS)
	}
	return strings.Join(parts, "\n")
}
