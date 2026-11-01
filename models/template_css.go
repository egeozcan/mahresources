package models

// TemplateCSSParts lists the shared and per-slot global styles in their stable
// cascade order. The template engine joins them (shortcodes.JoinCSS), removing
// each one's template comments on its own first.
func (c Category) TemplateCSSParts() []string {
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
	return parts
}
func (c ResourceCategory) TemplateCSSParts() []string {
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
	return parts
}
func (c NoteType) TemplateCSSParts() []string {
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
	return parts
}
