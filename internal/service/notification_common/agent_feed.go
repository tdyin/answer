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
	"sync"

	"github.com/apache/answer/internal/entity"
)

// AgentFeedRepo is the persistent unread feed plus process-local wakeups. Wakeups
// are hints, never acknowledgements. Consumers subscribe before taking a snapshot.
type AgentFeedRepo interface {
	AgentUnreadPage(context.Context, string, string, string, int) ([]*entity.Notification, string, error)
	SubscribeAgent(string) (<-chan string, func())
}

// AgentEventBus bounds each subscriber's backlog. A slow consumer is disconnected
// so it can reconcile against the authoritative unread rows instead of losing IDs.
type AgentEventBus struct {
	mu          sync.Mutex
	subscribers map[string]map[chan string]struct{}
}

func (b *AgentEventBus) Subscribe(userID string) (<-chan string, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subscribers == nil {
		b.subscribers = make(map[string]map[chan string]struct{})
	}
	if b.subscribers[userID] == nil {
		b.subscribers[userID] = make(map[chan string]struct{})
	}
	ch := make(chan string, 128)
	b.subscribers[userID][ch] = struct{}{}
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.remove(userID, ch)
	}
}

func (b *AgentEventBus) remove(userID string, ch chan string) {
	if _, exists := b.subscribers[userID][ch]; !exists {
		return
	}
	delete(b.subscribers[userID], ch)
	if len(b.subscribers[userID]) == 0 {
		delete(b.subscribers, userID)
	}
	close(ch)
}

func (b *AgentEventBus) Publish(userID, notificationID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers[userID] {
		select {
		case ch <- notificationID:
		default:
			b.remove(userID, ch)
		}
	}
}
