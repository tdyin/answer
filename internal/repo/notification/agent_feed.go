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

	"github.com/apache/answer/internal/base/reason"
	"github.com/apache/answer/internal/entity"
	"github.com/apache/answer/internal/schema"
	"github.com/segmentfault/pacman/errors"
)

func (nr *notificationRepo) SubscribeAgent(userID string) (<-chan string, func()) {
	return nr.events.Subscribe(userID)
}

// AgentUnreadPage uses an immutable ID boundary, not mutable updated_at or offsets.
// Acknowledgements between pages cannot shift subsequent rows out of the traversal.
func (nr *notificationRepo) AgentUnreadPage(ctx context.Context, userID, after, through string, limit int) ([]*entity.Notification, string, error) {
	rows := make([]*entity.Notification, 0)
	if through == "" {
		latest := &entity.Notification{}
		exists, err := nr.data.DB.Context(ctx).Where("user_id = ?", userID).Desc("id").Get(latest)
		if err != nil {
			return nil, "", errors.InternalServer(reason.DatabaseError).WithError(err)
		}
		through = "0"
		if exists {
			through = latest.ID
		}
	}
	err := nr.data.DB.Context(ctx).Where("user_id = ?", userID).
		And("type = ?", schema.NotificationTypeInbox).
		And("status = ?", schema.NotificationStatusNormal).
		And("is_read = ?", schema.NotificationNotRead).
		And("id > ?", after).And("id <= ?", through).
		Asc("id").Limit(limit).Find(&rows)
	if err != nil {
		return nil, "", errors.InternalServer(reason.DatabaseError).WithError(err)
	}
	return rows, through, nil
}
