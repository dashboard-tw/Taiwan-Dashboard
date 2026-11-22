package screen

import (
	"context"
	"time"
)

type Typhoon struct {
	ID       string     `json:"id"` // 2026-33
	TyNo     *int       `json:"ty_no"`
	NameZh   *string    `json:"name_zh"`
	NameEn   *string    `json:"name_en"`
	StartsAt time.Time  `json:"starts_at"`         // first warning
	EndsAt   *time.Time `json:"ends_at,omitempty"` // warning lifted; nil while in force
	Active   bool       `json:"active"`
	Alert    string     `json:"last_alert"`
	Warnings int        `json:"warnings"`
}

var typhoonDatasets = []string{"typhoon_warning", "typhoon_track"}

func (s *Service) Typhoons(ctx context.Context) ([]Typhoon, error) {
	list, _, err := s.typhoons.Get(ctx, "typhoons", typhoonDatasets, 5*time.Minute,
		func(ctx context.Context) ([]Typhoon, error) {
			rows, err := s.App.QueryContext(ctx, `
				SELECT typhoon_id, ty_no, name_zh, name_en, starts_at, ends_at, last_alert, warnings
				FROM data.typhoon_event
				ORDER BY starts_at DESC`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			out := []Typhoon{}
			for rows.Next() {
				var t Typhoon
				if err := rows.Scan(&t.ID, &t.TyNo, &t.NameZh, &t.NameEn, &t.StartsAt, &t.EndsAt, &t.Alert, &t.Warnings); err != nil {
					return nil, err
				}
				t.Active = t.EndsAt == nil
				out = append(out, t)
			}
			return out, rows.Err()
		})
	return list, err
}
