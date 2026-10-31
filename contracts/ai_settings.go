package contracts

// AIGenerationProfiles describes effective generation routing without credentials.
type AIGenerationProfiles struct {
	MRQL     AIGenerationProfile `json:"mrql"`
	Template AIGenerationProfile `json:"template"`
}

type AIGenerationProfile struct {
	Provider       string                    `json:"provider"`
	Model          string                    `json:"model"`
	Configured     bool                      `json:"configured"`
	MaxTokens      int                       `json:"maxTokens"`
	Timeout        string                    `json:"timeout"`
	BundleDeadline string                    `json:"bundleDeadline,omitempty"`
	Profiles       []AIGenerationCallProfile `json:"profiles,omitempty"`
}

type AIGenerationCallProfile struct {
	Kind          string `json:"kind"`
	Model         string `json:"model"`
	ReasoningMode string `json:"reasoningMode"`
	MaxTokens     int    `json:"maxTokens"`
	Timeout       string `json:"timeout"`
}
