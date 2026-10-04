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

package review

import (
	"context"
	"github.com/apache/answer/internal/base/constant"
	"github.com/apache/answer/internal/schema"
	"testing"
)

type selfNotificationQueue struct{ messages []*schema.NotificationMsg }

func (q *selfNotificationQueue) Send(_ context.Context, msg *schema.NotificationMsg) {
	q.messages = append(q.messages, msg)
}
func (q *selfNotificationQueue) Close() {}
func (q *selfNotificationQueue) RegisterHandler(func(context.Context, *schema.NotificationMsg) error) {
}

func TestSelfAnswerStillDispatchesToTopicWatchers(t *testing.T) {
	q := &selfNotificationQueue{}
	service := &ReviewService{notificationQueueService: q}
	service.notificationAnswerTheQuestion(context.Background(), "1", "10010000000000001", "10020000000000001", "1", "Topic", "Answer")
	if len(q.messages) != 1 {
		t.Fatalf("missing self-answer fanout seed: %d", len(q.messages))
	}
	msg := q.messages[0]
	if msg.NotificationAction != constant.NotificationAnswerTheQuestion || msg.ObjectID != "10020000000000001" || msg.TriggerUserID != "1" || msg.ReceiverUserID != "1" {
		t.Fatalf("incorrect self-answer seed: %+v", msg)
	}
}
