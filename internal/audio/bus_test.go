package audio

import (
	"testing"
	"time"
)

func makeFrame(systemID, tgid int) AudioFrame {
	return AudioFrame{
		SystemID:  systemID,
		TGID:      tgid,
		UnitID:    100,
		Seq:       1,
		Timestamp: 1000,
		Format:    AudioFormatPCM,
		Data:      []byte{0x01, 0x02, 0x03, 0x04},
	}
}

func TestAudioBusPublishToSubscriber(t *testing.T) {
	bus := NewAudioBus()
	ch, cancel := bus.Subscribe(AudioFilter{TGIDs: []int{1001}})
	defer cancel()

	frame := makeFrame(1, 1001)
	bus.Publish(frame)

	select {
	case got := <-ch:
		if got.SystemID != 1 {
			t.Errorf("SystemID = %d, want 1", got.SystemID)
		}
		if got.TGID != 1001 {
			t.Errorf("TGID = %d, want 1001", got.TGID)
		}
		if got.UnitID != 100 {
			t.Errorf("UnitID = %d, want 100", got.UnitID)
		}
		if got.Seq != 1 {
			t.Errorf("Seq = %d, want 1", got.Seq)
		}
		if got.Format != AudioFormatPCM {
			t.Errorf("Format = %v, want PCM", got.Format)
		}
		if len(got.Data) != 4 {
			t.Errorf("Data length = %d, want 4", len(got.Data))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for frame")
	}
}

func TestAudioBusFilterByTGID(t *testing.T) {
	bus := NewAudioBus()
	ch, cancel := bus.Subscribe(AudioFilter{TGIDs: []int{1001}})
	defer cancel()

	// Publish frame with wrong TGID
	bus.Publish(makeFrame(1, 2002))

	select {
	case <-ch:
		t.Fatal("received frame that should have been filtered out")
	case <-time.After(200 * time.Millisecond):
		// Expected: no frame received
	}
}

func TestAudioBusFilterBySystem(t *testing.T) {
	bus := NewAudioBus()
	ch, cancel := bus.Subscribe(AudioFilter{SystemIDs: []int{1}, TGIDs: []int{1001}})
	defer cancel()

	// Publish with wrong system
	bus.Publish(makeFrame(2, 1001))

	select {
	case <-ch:
		t.Fatal("received frame with wrong system ID")
	case <-time.After(200 * time.Millisecond):
		// Expected: filtered out
	}

	// Publish with correct system
	bus.Publish(makeFrame(1, 1001))

	select {
	case got := <-ch:
		if got.SystemID != 1 || got.TGID != 1001 {
			t.Errorf("got SystemID=%d TGID=%d, want 1/1001", got.SystemID, got.TGID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for matching frame")
	}
}

func TestAudioBusEmptyFilterReceivesAll(t *testing.T) {
	bus := NewAudioBus()
	ch, cancel := bus.Subscribe(AudioFilter{})
	defer cancel()

	bus.Publish(makeFrame(1, 1001))
	bus.Publish(makeFrame(2, 2002))

	received := 0
	timeout := time.After(500 * time.Millisecond)
	for received < 2 {
		select {
		case <-ch:
			received++
		case <-timeout:
			t.Fatalf("received %d frames, want 2", received)
		}
	}
}

func TestAudioBusCancelUnsubscribes(t *testing.T) {
	bus := NewAudioBus()
	ch, cancel := bus.Subscribe(AudioFilter{})

	cancel()

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Fatal("channel should be closed after cancel")
	}

	// Subscriber count should be 0
	if n := bus.SubscriberCount(); n != 0 {
		t.Errorf("SubscriberCount = %d, want 0", n)
	}

	// Publish should not panic
	bus.Publish(makeFrame(1, 1001))
}

func TestAudioBusSlowSubscriberDropsFrames(t *testing.T) {
	bus := NewAudioBus()
	ch, cancel := bus.Subscribe(AudioFilter{})
	defer cancel()

	// Publish 300 frames without reading (buffer is 256)
	for i := 0; i < 300; i++ {
		bus.Publish(makeFrame(1, 1001))
	}

	// Drain the channel
	drained := 0
	for {
		select {
		case <-ch:
			drained++
		default:
			goto done
		}
	}
done:

	if drained == 0 {
		t.Fatal("should have received some frames")
	}
	if drained > audioSubscriberBuffer {
		t.Errorf("drained %d frames, should be at most %d (buffer size)", drained, audioSubscriberBuffer)
	}
	t.Logf("drained %d/%d frames (buffer=%d)", drained, 300, audioSubscriberBuffer)
}

func TestAudioBusMultipleSubscribers(t *testing.T) {
	bus := NewAudioBus()

	ch1, cancel1 := bus.Subscribe(AudioFilter{TGIDs: []int{1001}})
	defer cancel1()
	ch2, cancel2 := bus.Subscribe(AudioFilter{TGIDs: []int{1001}})
	defer cancel2()

	bus.Publish(makeFrame(1, 1001))

	for i, ch := range []<-chan AudioFrame{ch1, ch2} {
		select {
		case got := <-ch:
			if got.TGID != 1001 {
				t.Errorf("subscriber %d: TGID = %d, want 1001", i, got.TGID)
			}
		case <-time.After(200 * time.Millisecond):
			t.Errorf("subscriber %d: timed out", i)
		}
	}

	if n := bus.SubscriberCount(); n != 2 {
		t.Errorf("SubscriberCount = %d, want 2", n)
	}
}

func TestAudioBusUpdateFilter(t *testing.T) {
	bus := NewAudioBus()
	ch, cancel := bus.Subscribe(AudioFilter{TGIDs: []int{1001}})
	defer cancel()

	// Update filter to TGID 2002
	bus.UpdateFilter(ch, AudioFilter{TGIDs: []int{2002}})

	// Old TGID should not match
	bus.Publish(makeFrame(1, 1001))

	select {
	case <-ch:
		t.Fatal("received frame for old TGID after filter update")
	case <-time.After(200 * time.Millisecond):
		// Expected
	}

	// New TGID should match
	bus.Publish(makeFrame(1, 2002))

	select {
	case got := <-ch:
		if got.TGID != 2002 {
			t.Errorf("TGID = %d, want 2002", got.TGID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for frame with updated filter")
	}
}
