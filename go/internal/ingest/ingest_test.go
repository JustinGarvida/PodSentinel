package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"podsentinel/internal/k8s"
	"podsentinel/internal/store"
)

type fakePoller struct {
	samples []k8s.PodSample
}

func (f fakePoller) Poll(ctx context.Context) []k8s.PodSample {
	return f.samples
}

type fakeStore struct {
	inserted []store.PodMetricRow
	failFor  string // pod name to fail insert for
}

func (f *fakeStore) InsertPodMetric(ctx context.Context, row store.PodMetricRow) error {
	if row.Pod == f.failFor {
		return errors.New("simulated insert failure")
	}
	f.inserted = append(f.inserted, row)
	return nil
}

type fakePublisher struct {
	published []k8s.PodSample
	failFor   string // pod name to fail publish for
}

func (f *fakePublisher) Publish(ctx context.Context, sample k8s.PodSample) error {
	if sample.Name == f.failFor {
		return errors.New("simulated publish failure")
	}
	f.published = append(f.published, sample)
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRun_PersistsEverySample(t *testing.T) {
	now := time.Now()
	poller := fakePoller{samples: []k8s.PodSample{
		{Namespace: "default", Name: "web-1", Status: "Running", CPU: 0.1, Memory: 1e8, Timestamp: now},
		{Namespace: "default", Name: "web-2", Status: "Running", CPU: 0.2, Memory: 2e8, Timestamp: now},
	}}
	st := &fakeStore{}

	Run(context.Background(), poller, st, nil, testLogger())

	if len(st.inserted) != 2 {
		t.Fatalf("len(inserted) = %d, want 2", len(st.inserted))
	}
}

func TestRun_SkipsFailedInsertWithoutStoppingTheRest(t *testing.T) {
	now := time.Now()
	poller := fakePoller{samples: []k8s.PodSample{
		{Namespace: "default", Name: "bad-pod", Status: "Running", Timestamp: now},
		{Namespace: "default", Name: "good-pod", Status: "Running", Timestamp: now},
	}}
	st := &fakeStore{failFor: "bad-pod"}

	Run(context.Background(), poller, st, nil, testLogger())

	if len(st.inserted) != 1 || st.inserted[0].Pod != "good-pod" {
		t.Fatalf("inserted = %+v, want only good-pod to have been persisted", st.inserted)
	}
}

func TestRun_PassesThroughUIDAndOwnerFromSample(t *testing.T) {
	now := time.Now()
	poller := fakePoller{samples: []k8s.PodSample{
		{
			Namespace: "default", Name: "web-7d9f8c6b5d-x8k2p", Status: "Running",
			UID: "pod-uid-123", OwnerKind: "Deployment", OwnerName: "web",
			Timestamp: now,
		},
	}}
	st := &fakeStore{}

	Run(context.Background(), poller, st, nil, testLogger())

	if len(st.inserted) != 1 {
		t.Fatalf("len(inserted) = %d, want 1", len(st.inserted))
	}
	got := st.inserted[0]
	if got.PodUID != "pod-uid-123" || got.OwnerKind != "Deployment" || got.OwnerName != "web" {
		t.Errorf("row = %+v, want PodUID=pod-uid-123 OwnerKind=Deployment OwnerName=web", got)
	}
}

func TestRun_PublishesEverySample(t *testing.T) {
	now := time.Now()
	poller := fakePoller{samples: []k8s.PodSample{
		{Namespace: "default", Name: "web-1", Timestamp: now},
		{Namespace: "default", Name: "web-2", Timestamp: now},
	}}
	pub := &fakePublisher{}

	Run(context.Background(), poller, &fakeStore{}, pub, testLogger())

	if len(pub.published) != 2 {
		t.Fatalf("len(published) = %d, want 2", len(pub.published))
	}
}

func TestRun_PublishFailureDoesNotStopInsertsOrOtherPublishes(t *testing.T) {
	now := time.Now()
	poller := fakePoller{samples: []k8s.PodSample{
		{Namespace: "default", Name: "bad-pod", Timestamp: now},
		{Namespace: "default", Name: "good-pod", Timestamp: now},
	}}
	st := &fakeStore{}
	pub := &fakePublisher{failFor: "bad-pod"}

	Run(context.Background(), poller, st, pub, testLogger())

	if len(st.inserted) != 2 {
		t.Errorf("len(inserted) = %d, want 2 — publish failure must not skip the insert", len(st.inserted))
	}
	if len(pub.published) != 1 || pub.published[0].Name != "good-pod" {
		t.Errorf("published = %+v, want only good-pod", pub.published)
	}
}

func TestRun_InsertFailureStillPublishes(t *testing.T) {
	now := time.Now()
	poller := fakePoller{samples: []k8s.PodSample{
		{Namespace: "default", Name: "bad-pod", Timestamp: now},
	}}
	pub := &fakePublisher{}

	Run(context.Background(), poller, &fakeStore{failFor: "bad-pod"}, pub, testLogger())

	if len(pub.published) != 1 {
		t.Errorf("len(published) = %d, want 1 — insert failure must not skip the publish", len(pub.published))
	}
}
