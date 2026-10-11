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

package notificationcommon

import (
	"context"
	"github.com/apache/answer/internal/base/constant"
	"github.com/apache/answer/internal/base/queue"
	"github.com/apache/answer/internal/entity"
	"github.com/apache/answer/internal/schema"
	"github.com/apache/answer/internal/service/activity_common"
	"github.com/apache/answer/internal/service/object_info"
	questioncommon "github.com/apache/answer/internal/service/question_common"
	"testing"
	"time"
)

type fanoutQuestionRepo struct{ questioncommon.QuestionRepo }

func (*fanoutQuestionRepo) GetQuestion(context.Context, string) (*entity.Question, bool, error) {
	return &entity.Question{ID: "10010000000000001", UserID: "1"}, true, nil
}

type blockingFollowRepo struct {
	activity_common.FollowRepo
	entered, release chan struct{}
}

func (r *blockingFollowRepo) GetFollowUserIDs(context.Context, string) ([]string, error) {
	close(r.entered)
	<-r.release
	return nil, nil
}
func TestQueueCloseWaitsForFanoutOnlySeed(t *testing.T) {
	follows := &blockingFollowRepo{entered: make(chan struct{}), release: make(chan struct{})}
	q := queue.New[*schema.NotificationMsg]("fanout-shutdown", 1)
	service := &NotificationCommon{followRepo: follows, notificationQueueService: q, objectInfoService: object_info.NewObjService(nil, &fanoutQuestionRepo{}, nil, nil, nil)}
	q.RegisterHandler(service.AddNotification)
	q.Send(context.Background(), &schema.NotificationMsg{ObjectID: "10010000000000001", NotificationAction: constant.NotificationAnswerTheQuestion, Type: schema.NotificationTypeInbox, TriggerUserID: "1", ReceiverUserID: "1", OnlyPushAllFollow: true})
	select {
	case <-follows.entered:
	case <-time.After(time.Second):
		t.Fatal("fanout did not start")
	}
	done := make(chan struct{})
	go func() { q.Close(); close(done) }()
	select {
	case <-done:
		close(follows.release)
		t.Fatal("Close returned before fanout completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(follows.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close did not complete after fanout")
	}
}
