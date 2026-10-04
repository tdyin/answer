/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package notification

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/apache/answer/internal/base/data"
	"github.com/apache/answer/internal/entity"
	"github.com/apache/answer/internal/schema"
	"xorm.io/xorm"
)

func TestAgentFeedSnapshotAcknowledgementsAndLiveOverlap(t *testing.T) {
	db, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := db.Sync2(new(entity.Notification)); err != nil {
		t.Fatal(err)
	}
	repo := &notificationRepo{data: &data.Data{DB: db}}
	ctx := context.Background()
	events, cancel := repo.SubscribeAgent("1")
	defer cancel()
	add := func(user string, read, status, kind int) string {
		t.Helper()
		row := &entity.Notification{UserID: user, ObjectID: "100", Content: "{}", Type: kind, IsRead: read, Status: status}
		if err := repo.AddNotification(ctx, row); err != nil {
			t.Fatal(err)
		}
		if row.ID == "" {
			t.Fatal("missing persisted ID")
		}
		return row.ID
	}
	first := add("1", 1, 1, 1)
	second := add("1", 1, 1, 1)
	third := add("1", 1, 1, 1)
	add("2", 1, 1, 1)  // other recipient
	add("1", 2, 1, 1)  // already read
	add("1", 1, 10, 1) // deleted
	add("1", 1, 1, 2)  // achievement
	rows, boundary, err := repo.AgentUnreadPage(ctx, "1", "0", "", 2)
	if err != nil || len(rows) != 2 || rows[0].ID != first || rows[1].ID != second {
		t.Fatalf("first page: %+v %v", rows, err)
	}
	if err := repo.ClearIDUnRead(ctx, "2", third); err != nil {
		t.Fatal(err)
	} // wrong principal cannot acknowledge
	if err := repo.ClearIDUnRead(ctx, "1", first); err != nil {
		t.Fatal(err)
	}
	later := add("1", 1, 1, 1)
	rows, sameBoundary, err := repo.AgentUnreadPage(ctx, "1", second, boundary, 2)
	if err != nil || sameBoundary != boundary || len(rows) != 1 || rows[0].ID != third {
		t.Fatalf("second page: %+v %v", rows, err)
	}
	rows, _, err = repo.AgentUnreadPage(ctx, "1", boundary, "", 100)
	if err != nil || len(rows) != 1 || rows[0].ID != later {
		t.Fatalf("post-snapshot page: %+v %v", rows, err)
	}
	for _, want := range []string{first, second, third} {
		if got := <-events; got != want {
			t.Fatalf("live ID %s, expected %s", got, want)
		}
	}
	// Reading either transport must not acknowledge delivery.
	row, found, err := repo.GetById(ctx, third)
	if err != nil || !found || row.IsRead != schema.NotificationNotRead {
		t.Fatalf("delivery changed unread state: %+v %v", row, err)
	}
}

func TestAcknowledgementIsRecipientScopedAndAtomic(t *testing.T) {
	db, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := db.Sync2(new(entity.Notification)); err != nil {
		t.Fatal(err)
	}
	repo := &notificationRepo{data: &data.Data{DB: db}}
	row := &entity.Notification{UserID: "1", ObjectID: "100", Content: "{}", Type: 1, IsRead: 1, Status: 1}
	if err := repo.AddNotification(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	if changed, err := repo.AcknowledgeNotification(context.Background(), "2", row.ID); err != nil || changed {
		t.Fatalf("other recipient: %v %v", changed, err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			changed, err := repo.AcknowledgeNotification(context.Background(), "1", row.ID)
			if err != nil {
				t.Error(err)
			}
			if changed {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("expected exactly one read transition, got %d", winners.Load())
	}
}
