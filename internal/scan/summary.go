package scan

// Summary counts entries per status.
type Summary struct {
	Candidates         int `json:"candidates"`
	PartiallyStowed    int `json:"partiallyStowed"`
	AlreadyStowed      int `json:"alreadyStowed"`
	LinkedElsewhere    int `json:"linkedElsewhere"`
	NoConfigFiles      int `json:"noConfigFiles"`
	TooManyConfigFiles int `json:"tooManyConfigFiles"`
	TooManySubdirs     int `json:"tooManySubdirs"`
	Total              int `json:"total"`
}

// Summarize counts entries by status.
func Summarize(entries []Entry) Summary {
	summary := Summary{Total: len(entries)}
	for _, entry := range entries {
		switch entry.Status {
		case Candidate:
			summary.Candidates++
		case PartiallyStowed:
			summary.PartiallyStowed++
		case AlreadyStowed:
			summary.AlreadyStowed++
		case LinkedElsewhere:
			summary.LinkedElsewhere++
		case NoConfigFiles:
			summary.NoConfigFiles++
		case TooManyConfigFiles:
			summary.TooManyConfigFiles++
		case TooManySubdirs:
			summary.TooManySubdirs++
		}
	}
	return summary
}
