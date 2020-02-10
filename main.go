// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goissues exports issues from the golang/go project (via the Maintner mirror
// service) to CSV for analysis.
//
// Forked from github.com/bcmills/goissues.
package main

import (
	"context"
	"encoding/csv"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/build/maintner"
	"golang.org/x/build/maintner/godata"
)

// GitHub label IDs.
//
// Extract using:
// 	curl -sn https://api.github.com/repos/golang/go/labels/$LABELNAME | jq .id
const (
	proposalID       = 236419512
	proposalHoldID   = 477156222
	needsDecisionID  = 373401956
	frozenDueToAgeID = 398069301
	waitingForInfoID = 357033853
)

func main() {
	corpus, err := godata.Get(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	project := corpus.Gerrit().Project("go.googlesource.com", "go")
	if project == nil {
		log.Fatal("go.googlesource.com/go not found")
	}

	repo := corpus.GitHub().Repo("golang", "go")
	if repo == nil {
		log.Fatal("github.com/golang/go not found")
	}

	issueHasCL := map[int32]bool{}
	err = project.ForeachOpenCL(func(cl *maintner.GerritCL) error {
		switch cl.Status {
		case "merged", "abandoned":
			return nil
		}
		hasRef := false
		for _, ref := range cl.GitHubIssueRefs {
			if ref.Repo == repo {
				hasRef = true
				break
			}
		}
		if !hasRef {
			return nil
		}
		if len(cl.Metas) >= 1 {
			meta := cl.Metas[len(cl.Metas)-1]
			for _, vote := range meta.LabelVotes()["Code-Review"] {
				if vote == -2 {
					return nil
				}
			}
		}
		for _, ref := range cl.GitHubIssueRefs {
			if ref.Repo == repo {
				issueHasCL[ref.Number] = true
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	w := csv.NewWriter(os.Stdout)

	var who, labels []string
	err = repo.ForeachIssue(func(i *maintner.GitHubIssue) error {
		who = who[:0]
		labels = labels[:0]
		if i.NotExist || i.PullRequest || (i.Locked && i.HasLabelID(frozenDueToAgeID)) {
			return nil
		}

		number := strconv.FormatInt(int64(i.Number), 10)
		updated := i.Updated.Format("2006-01-02")

		state := ""
		switch {
		case i.Closed:
			state = "closed"
		case i.Locked:
			state = "locked"
		}

		milestone := ""
		if i.Milestone != nil {
			milestone = i.Milestone.Title
		}

		for _, l := range i.Labels {
			switch l.ID {
			case waitingForInfoID, proposalHoldID:
				switch state {
				case "", "deciding":
					state = "waiting"
				}
			case needsDecisionID:
				switch state {
				case "":
					state = "deciding"
				}
			default:
				labels = append(labels, strings.ToLower(l.Name))
			}
		}
		sort.Strings(labels)

		if state == "" {
			if issueHasCL[i.Number] {
				state = "pending"
			} else {
				state = "open"
			}
		}

		for _, a := range i.Assignees {
			if a.Login == "" {
				continue
			}
			who = append(who, a.Login)
		}
		sort.Strings(who)

		return w.Write([]string{number, updated, state, milestone, strings.Join(labels, ","), strings.Join(who, ","), i.Title})
	})
	if err != nil {
		log.Fatal(err)
	}

	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}
}
