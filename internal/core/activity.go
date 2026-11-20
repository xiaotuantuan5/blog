package core

import (
	"context"
	"time"
)

// Activity days use UTC+8 so the heatmap aligns with the user's evenings.
var cst = time.FixedZone("CST", 8*3600)

// recordActivity marks that noteID was created or edited today. A note only
// counts once per day and action.
func recordActivity(ctx context.Context, q querier, noteID int64, action string) error {
	day := time.Now().In(cst).Format("2006-01-02")
	_, err := q.ExecContext(ctx,
		`INSERT OR IGNORE INTO activity (day, note_id, action) VALUES (?, ?, ?)`, day, noteID, action)
	return err
}

// DayCount is one day's cell of the heatmap.
type DayCount struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

// Heatmap returns per-day distinct-note counts between from and to
// (inclusive, YYYY-MM-DD). Purged notes drop out automatically.
func (a *App) Heatmap(ctx context.Context, from, to string) ([]DayCount, error) {
	rows, err := a.DB.QueryContext(ctx, `
		SELECT day, COUNT(DISTINCT note_id) FROM activity
		WHERE day BETWEEN ? AND ? GROUP BY day ORDER BY day`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []DayCount{}
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Day, &d.Count); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}
