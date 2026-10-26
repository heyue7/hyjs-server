package model

// AdminGuildScheduleForm uses Beijing civil times; operators never enter epoch milliseconds.
type AdminGuildScheduleForm struct {
	Day             string `json:"day"`
	EnrollmentStart string `json:"enrollmentStart"`
	EnrollmentEnd   string `json:"enrollmentEnd"`
	FirstStartTime  string `json:"firstStartTime"`
	SecondStartTime string `json:"secondStartTime"`
	DurationMinutes int64  `json:"durationMinutes"`
	Enabled         bool   `json:"enabled"`
	Version         string `json:"version"`
}
type AdminGuildScheduleRow struct {
	AdminGuildScheduleForm
	RegistrationDay string `json:"registrationDay"`
	FirstEnd        string `json:"firstEnd"`
	SecondEnd       string `json:"secondEnd"`
	PublishedAt     int64  `json:"publishedAt"`
	RowVersion      string `json:"rowVersion"`
	Frozen          bool   `json:"frozen"`
	Editable        bool   `json:"editable"`
	Legacy          bool   `json:"legacy"`
	// PublicationNotes explains preserved switches or non-retroactive prizes after saving.
	PublicationNotes []string `json:"publicationNotes,omitempty"`
}
type AdminGuildSchedulesResponse struct {
	Items                 []AdminGuildScheduleRow `json:"items"`
	CustomCalendarEnabled bool                    `json:"customCalendarEnabled"`
	PolicyVersion         string                  `json:"policyVersion"`
	PreparationMinutes    float64                 `json:"preparationMinutes"`
}
type AdminGuildScheduleSaveRequest struct {
	AdminGuildScheduleForm
	RowVersion string `json:"rowVersion"`
	Reason     string `json:"reason"`
}
type AdminGuildCalendarSaveRequest struct {
	Enabled    bool   `json:"enabled"`
	RowVersion string `json:"rowVersion"`
	Reason     string `json:"reason"`
}
