package api

import (
	"log"

	"kanban_board/internal/runner"
)

// reconcileAcceptedRuns fixes data from before "accepted" existed: a card sitting in Concluído whose latest
// run is still "succeeded" was accepted already (direct mode, or its branch is gone). Runs whose branch still
// exists are left alone, because those changes were never merged.
func (s *Server) reconcileAcceptedRuns() {
	projects, err := s.store.ListProjects()
	if err != nil {
		return
	}
	fixed := 0
	for _, p := range projects {
		cards, err := s.store.ListCards(p.ID)
		if err != nil {
			continue
		}
		for _, c := range cards {
			if c.Column != "done" || c.RunStatus != "succeeded" {
				continue
			}
			run, err := s.store.LatestRun(c.ID)
			if err != nil || run.Status != "succeeded" {
				continue
			}
			if p.Isolation != "direct" && run.Branch != "" && runner.BranchExists(p.Path, run.Branch) {
				continue
			}
			if s.store.UpdateRun(run.ID, "accepted", "", "", "", "", nil, nil) == nil {
				fixed++
			}
		}
	}
	if fixed > 0 {
		log.Printf("%d execução(ões) de cards concluídos marcadas como aceitas", fixed)
	}
}
