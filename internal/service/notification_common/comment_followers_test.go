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
	"testing"

	"github.com/apache/answer/internal/base/constant"
	"github.com/apache/answer/internal/schema"
	"github.com/apache/answer/internal/service/activity_common"
	"github.com/apache/answer/internal/service/noticequeue"
)

type followerFixture struct {
	activity_common.FollowRepo
	question string
}

func (r *followerFixture) GetFollowUserIDs(_ context.Context, question string) ([]string, error) {
	r.question = question
	return []string{"1", "2", "3", "4", "4"}, nil
}

type queueFixture struct{ messages []*schema.NotificationMsg }

func (q *queueFixture) Send(_ context.Context, msg *schema.NotificationMsg) {
	q.messages = append(q.messages, msg)
}
func (q *queueFixture) Close()                                                               {}
func (q *queueFixture) RegisterHandler(func(context.Context, *schema.NotificationMsg) error) {}

func TestCommentFollowersExcludeActorDirectRecipientsAndDuplicateFollows(t *testing.T) {
	for _, parent := range []string{constant.QuestionObjectType, constant.AnswerObjectType} {
		queue := &queueFixture{}
		direct := map[string]bool{"1": true, "2": true, "3": true}
		noticequeue.SendCommentFollowers(context.Background(), queue, "10030000000000001", "1", parent, direct)
		if len(queue.messages) != 1 || !queue.messages[0].OnlyPushAllFollow {
			t.Fatal("expected one fanout-only seed")
		}
		seed := queue.messages[0]
		queue.messages = nil
		follows := &followerFixture{}
		service := &NotificationCommon{followRepo: follows, notificationQueueService: queue}
		service.SendNotificationToAllFollower(context.Background(), seed, "10010000000000001")
		if len(queue.messages) != 1 {
			t.Fatalf("expected only unrelated follower: %+v", queue.messages)
		}
		got := queue.messages[0]
		if got.ReceiverUserID != "4" || got.TriggerUserID != "1" || got.ObjectID != seed.ObjectID || !got.NoNeedPushAllFollow || got.OnlyPushAllFollow {
			t.Fatalf("invalid follower message: %+v", got)
		}
		if follows.question != "10010000000000001" {
			t.Fatal("did not resolve topic follows")
		}
		queue.messages = nil
		service.SendNotificationToAllFollower(context.Background(), got, follows.question)
		if len(queue.messages) != 0 {
			t.Fatal("recursive fanout")
		}
	}
}

func TestAnswerFollowersDoNotDuplicateOriginalRecipient(t *testing.T) {
	queue := &queueFixture{}
	service := &NotificationCommon{followRepo: &followerFixture{}, notificationQueueService: queue}
	service.SendNotificationToAllFollower(context.Background(), &schema.NotificationMsg{
		TriggerUserID: "1", ReceiverUserID: "2", NotificationAction: constant.NotificationAnswerTheQuestion,
	}, "10010000000000001")
	if len(queue.messages) != 2 || queue.messages[0].ReceiverUserID != "3" || queue.messages[1].ReceiverUserID != "4" {
		t.Fatalf("unexpected recipients: %+v", queue.messages)
	}
}
