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

package controller

import (
	"context"
	"errors"
	"github.com/apache/answer/internal/entity"
	"github.com/apache/answer/internal/service/notification"
	notificationcommon "github.com/apache/answer/internal/service/notification_common"
	usercommon "github.com/apache/answer/internal/service/user_common"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

type feedUserRepo struct {
	usercommon.UserRepo
	err    error
	user   *entity.User
	exists bool
}

func (r *feedUserRepo) GetByUserID(context.Context, string) (*entity.User, bool, error) {
	return r.user, r.exists, r.err
}
func TestAgentFeedRepositoryFailureIsRetryable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		service := notification.NewNotificationService(nil, nil, nil, nil, &feedUserRepo{err: errors.New("database unavailable")}, nil, nil, nil)
		controller := &NotificationController{notificationService: service}
		r := gin.New()
		if stream {
			r.GET("/feed", controller.AgentEvents)
		} else {
			r.GET("/feed", controller.AgentUnreadPage)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/feed", nil))
		if w.Code != 503 {
			t.Fatalf("stream=%v: got %d, want retryable 503", stream, w.Code)
		}
	}
}

type failingStreamUserRepo struct {
	usercommon.UserRepo
	calls int
}

func (r *failingStreamUserRepo) GetByUserID(context.Context, string) (*entity.User, bool, error) {
	r.calls++
	if r.calls == 1 {
		return &entity.User{Status: entity.UserStatusAvailable, MailStatus: entity.EmailStatusAvailable}, true, nil
	}
	return nil, false, errors.New("database unavailable")
}

type streamFeedRepo struct {
	notificationcommon.NotificationRepo
	cancelled bool
}

func (r *streamFeedRepo) SubscribeAgent(string) (<-chan string, func()) {
	ch := make(chan string, 1)
	ch <- "1"
	return ch, func() { r.cancelled = true }
}
func (*streamFeedRepo) AgentUnreadPage(context.Context, string, string, string, int) ([]*entity.Notification, string, error) {
	return nil, "", nil
}
func TestAgentStreamReportsRepositoryFailureAfterReady(t *testing.T) {
	repo := &streamFeedRepo{}
	service := notification.NewNotificationService(nil, repo, nil, nil, &failingStreamUserRepo{}, nil, nil, nil)
	controller := &NotificationController{notificationService: service}
	r := gin.New()
	r.GET("/feed", controller.AgentEvents)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/feed", nil))
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "event: ready") || !strings.Contains(body, `"code":"service_unavailable","retryable":true`) || strings.Contains(body, "event: notification") || !repo.cancelled {
		t.Fatalf("unexpected stream: %d %s cancelled=%v", w.Code, body, repo.cancelled)
	}
}
func TestAgentFeedMissingAccountRemainsForbidden(t *testing.T) {
	service := notification.NewNotificationService(nil, nil, nil, nil, &feedUserRepo{}, nil, nil, nil)
	controller := &NotificationController{notificationService: service}
	for _, stream := range []bool{false, true} {
		r := gin.New()
		if stream {
			r.GET("/feed", controller.AgentEvents)
		} else {
			r.GET("/feed", controller.AgentUnreadPage)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/feed", nil))
		if w.Code != 403 {
			t.Fatalf("got %d, want 403", w.Code)
		}
	}
}
