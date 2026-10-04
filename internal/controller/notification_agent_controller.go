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
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/apache/answer/internal/base/handler"
	"github.com/apache/answer/internal/base/middleware"
	"github.com/gin-gonic/gin"
)

func agentCursor(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	_, err := strconv.ParseInt(value, 10, 64)
	return err == nil
}

func (nc *NotificationController) AgentUnreadPage(ctx *gin.Context) {
	after, through := ctx.DefaultQuery("after", "0"), ctx.Query("through")
	limit, err := strconv.Atoi(ctx.DefaultQuery("limit", "100"))
	if !agentCursor(after) || (through != "" && !agentCursor(through)) || err != nil || limit < 1 || limit > 100 {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	userID := middleware.GetLoginUserIDFromContext(ctx)
	if !nc.notificationService.AgentMayReceive(ctx, userID) {
		ctx.AbortWithStatus(http.StatusForbidden)
		return
	}
	page, err := nc.notificationService.AgentUnreadPage(ctx, userID, after, through, limit)
	handler.HandleResponse(ctx, err, page)
}

// AgentEvents emits IDs only; consumers must read the permissioned unread feed.
// Subscribe precedes ready so a snapshot taken after ready overlaps all live changes.
func (nc *NotificationController) AgentEvents(ctx *gin.Context) {
	userID := middleware.GetLoginUserIDFromContext(ctx)
	if !nc.notificationService.AgentMayReceive(ctx, userID) {
		ctx.AbortWithStatus(http.StatusForbidden)
		return
	}
	events, cancel, err := nc.notificationService.SubscribeAgent(userID)
	if err != nil {
		ctx.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	defer cancel()
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("X-Accel-Buffering", "no")
	// Bound writes so disconnected/slow clients cannot retain a subscriber forever.
	controller := http.NewResponseController(ctx.Writer)
	write := func(data string) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := fmt.Fprint(ctx.Writer, data); err != nil {
			return false
		}
		if err := controller.Flush(); err != nil {
			return false
		}
		return ctx.Request.Context().Err() == nil
	}
	if !write("event: ready\ndata: {}\n\n") {
		return
	}
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case id, open := <-events:
			if !open || !nc.notificationService.AgentMayReceive(ctx, userID) {
				return
			}
			if !agentCursor(id) || !write("event: notification\ndata: {\"notificationId\":\""+id+"\"}\n\n") {
				return
			}
		case <-heartbeat.C:
			if !nc.notificationService.AgentMayReceive(ctx, userID) || !write(": heartbeat\n\n") {
				return
			}
		}
	}
}
