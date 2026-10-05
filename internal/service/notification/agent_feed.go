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
	"encoding/json"
	"fmt"

	"github.com/apache/answer/internal/base/constant"
	"github.com/apache/answer/internal/entity"
	"github.com/apache/answer/internal/schema"
	notificationcommon "github.com/apache/answer/internal/service/notification_common"
)

type AgentNotification struct {
	NotificationID string `json:"notificationId"`
	RecipientID    string `json:"recipientId"`
	ActorID        string `json:"actorId"`
	TopicID        string `json:"topicId"`
	ObjectID       string `json:"objectId"`
	Kind           string `json:"kind"`
}

type AgentNotificationPage struct {
	Events  []AgentNotification `json:"events"`
	After   string              `json:"after"`
	Through string              `json:"through"`
	HasMore bool                `json:"hasMore"`
}

// AgentMayReceive rechecks the current account, including for already-open streams.
func (ns *NotificationService) AgentMayReceive(ctx context.Context, userID string) (bool, error) {
	user, exists, err := ns.userRepo.GetByUserID(ctx, userID)
	if err != nil {
		return false, err
	}
	return exists && user != nil && user.Status == entity.UserStatusAvailable && user.MailStatus == entity.EmailStatusAvailable, nil
}

func (ns *NotificationService) SubscribeAgent(userID string) (<-chan string, func(), error) {
	repo, ok := ns.notificationRepo.(notificationcommon.AgentFeedRepo)
	if !ok {
		return nil, nil, fmt.Errorf("agent notification feed unavailable")
	}
	events, cancel := repo.SubscribeAgent(userID)
	return events, cancel, nil
}

func (ns *NotificationService) AgentUnreadPage(ctx context.Context, userID, after, through string, limit int) (*AgentNotificationPage, error) {
	repo, ok := ns.notificationRepo.(notificationcommon.AgentFeedRepo)
	if !ok {
		return nil, fmt.Errorf("agent notification feed unavailable")
	}
	rows, boundary, err := repo.AgentUnreadPage(ctx, userID, after, through, limit)
	if err != nil {
		return nil, err
	}
	page := &AgentNotificationPage{Events: make([]AgentNotification, 0), After: after, Through: boundary, HasMore: len(rows) == limit}
	for _, row := range rows {
		page.After = row.ID // Advance even past unsupported notification actions.
		if event, ok := agentNotification(row); ok {
			page.Events = append(page.Events, event)
		}
	}
	return page, nil
}

func agentNotification(row *entity.Notification) (AgentNotification, bool) {
	var content schema.NotificationContent
	if json.Unmarshal([]byte(row.Content), &content) != nil || content.UserInfo == nil {
		return AgentNotification{}, false
	}
	kind := ""
	switch content.NotificationAction {
	case constant.NotificationAnswerTheQuestion:
		kind = "answer.created"
	case constant.NotificationCommentQuestion, constant.NotificationCommentAnswer, constant.NotificationReplyToYou:
		kind = "comment.created"
	case constant.NotificationMentionYou:
		kind = "mention"
	case constant.NotificationAcceptAnswer:
		kind = "topic.resolved"
	default:
		return AgentNotification{}, false
	}
	event := AgentNotification{NotificationID: row.ID, RecipientID: row.UserID, ActorID: content.UserInfo.ID,
		TopicID: content.ObjectInfo.ObjectMap["question"], ObjectID: content.ObjectInfo.ObjectID, Kind: kind}
	if event.NotificationID == "" || event.ActorID == "" || event.RecipientID == "" || event.ActorID == event.RecipientID || event.TopicID == "" || event.ObjectID == "" {
		return AgentNotification{}, false
	}
	return event, true
}
