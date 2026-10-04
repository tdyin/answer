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
	"fmt"
	"sync"
	"testing"
)

func TestAgentEventBusIsolationAndOverflow(t *testing.T) {
	var bus AgentEventBus
	alice, cancelAlice := bus.Subscribe("alice")
	defer cancelAlice()
	bob, cancelBob := bus.Subscribe("bob")
	defer cancelBob()
	for i := 0; i < 129; i++ {
		bus.Publish("alice", fmt.Sprint(i))
	}
	for i := 0; i < 128; i++ {
		if got, ok := <-alice; !ok || got != fmt.Sprint(i) {
			t.Fatalf("lost ordered ID %d: %q %v", i, got, ok)
		}
	}
	if _, ok := <-alice; ok {
		t.Fatal("overflow must disconnect for resync")
	}
	select {
	case <-bob:
		t.Fatal("cross-user event")
	default:
	}
	bus.Publish("bob", "130")
	if got := <-bob; got != "130" {
		t.Fatal(got)
	}
	cancelAlice() // cancellation remains safe after overflow
	cancelBob()
	cancelBob()
	if _, ok := <-bob; ok {
		t.Fatal("cancellation must close subscriber")
	}
}

func TestAgentEventBusConcurrentCancellation(t *testing.T) {
	var bus AgentEventBus
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, cancel := bus.Subscribe("alice")
			bus.Publish("alice", "1")
			cancel()
		}()
	}
	wg.Wait()
	if len(bus.subscribers) != 0 {
		t.Fatal("subscriber leak")
	}
}
