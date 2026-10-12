package agents

import "strings"

type Agent struct {
	ID, PaneID, WorkspaceID, WorkspaceName, TabID string
	Name, Provider, Status, CWD, Activity         string
	Description                                   string
	Icon, Color                                   string
	Custom                                        bool
	Favorite                                      bool
}

func MapStatus(s string) string {
	switch strings.ToLower(s) {
	case "working":
		return "WORKING"
	case "blocked":
		return "WAITING"
	case "done":
		return "DONE"
	case "idle":
		return "IDLE"
	case "unknown":
		return "UNKNOWN"
	default:
		return "OFFLINE"
	}
}

func Merge(live []Agent, custom []Agent) []Agent {
	byID := make(map[string]Agent, len(live))
	for _, a := range live {
		byID[a.ID] = a
	}
	for _, c := range custom {
		if a, ok := byID[c.ID]; ok {
			a.Name = c.Name
			a.Icon = c.Icon
			a.Color = c.Color
			a.Description = c.Description
			a.Favorite = c.Favorite
			a.Custom = true
			byID[c.ID] = a
		}
	}
	out := make([]Agent, 0, len(byID)+len(custom))
	for _, a := range live {
		out = append(out, byID[a.ID])
	}
	for _, c := range custom {
		if _, ok := byID[c.ID]; !ok {
			c.Custom = true
			c.Status = "OFFLINE"
			out = append(out, c)
		}
	}
	return out
}
