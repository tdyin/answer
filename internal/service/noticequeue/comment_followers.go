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

package noticequeue

import (
	"context"

	"github.com/apache/answer/internal/base/constant"
	"github.com/apache/answer/internal/schema"
)

// SendCommentFollowers emits one fan-out seed after direct recipients have been
// resolved. It also runs for a comment by the parent author, with no direct inbox
// recipient. The caller defers this until its recipient set is complete.
func SendCommentFollowers(ctx context.Context, queue Service, commentID, actorID, parentType string, notified map[string]bool) {
	action := constant.NotificationCommentQuestion
	if parentType == constant.AnswerObjectType {
		action = constant.NotificationCommentAnswer
	}
	excluded := make([]string, 0, len(notified))
	for userID := range notified {
		excluded = append(excluded, userID)
	}
	queue.Send(ctx, &schema.NotificationMsg{
		TriggerUserID: actorID, Type: schema.NotificationTypeInbox,
		ObjectID: commentID, ObjectType: constant.CommentObjectType,
		NotificationAction: action, OnlyPushAllFollow: true, ExcludeFollowerUserIDs: excluded,
	})
}
