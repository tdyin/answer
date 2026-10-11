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
	"encoding/json"
	"testing"

	"github.com/apache/answer/internal/base/constant"
	"github.com/apache/answer/internal/entity"
	"github.com/apache/answer/internal/schema"
)

func TestAgentNotificationOnlyRoutesSupportedExternalEvents(t *testing.T) {
	content := schema.NotificationContent{UserInfo: &schema.UserBasicInfo{ID: "2"},
		ObjectInfo: schema.ObjectInfo{ObjectID: "3", ObjectMap: map[string]string{"question": "4"}, Title: "untrusted title"}}
	for action, kind := range map[string]string{
		constant.NotificationAnswerTheQuestion: "answer.created",
		constant.NotificationCommentQuestion:   "comment.created",
		constant.NotificationReplyToYou:        "comment.created",
		constant.NotificationCommentAnswer:     "comment.created",
		constant.NotificationMentionYou:        "mention",
		constant.NotificationAcceptAnswer:      "topic.resolved",
		"other":                                "",
	} {
		content.NotificationAction = action
		raw, _ := json.Marshal(content)
		row := &entity.Notification{ID: "5", UserID: "1", Content: string(raw)}
		event, ok := agentNotification(row)
		if ok != (kind != "") || event.Kind != kind {
			t.Fatalf("%s: %+v %v", action, event, ok)
		}
		if ok && (event.NotificationID != "5" || event.ActorID != "2" || event.RecipientID != "1" || event.TopicID != "4" || event.ObjectID != "3") {
			t.Fatal(event)
		}
		row.UserID = "2"
		if _, ok := agentNotification(row); ok {
			t.Fatal("self event")
		}
	}
	for _, invalid := range []string{"bad", "{}", `{"user_info":{"id":"2"}}`} {
		if _, ok := agentNotification(&entity.Notification{ID: "5", UserID: "1", Content: invalid}); ok {
			t.Fatal("malformed event accepted")
		}
	}
}
