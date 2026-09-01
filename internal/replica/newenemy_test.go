package replica

import "testing"

// TestNewEnemyStaleReplicaBlocked is the A3 gate: revoke → fence → publish
// cannot ALLOW using an evaluation revision below the fence.
func TestNewEnemyStaleReplicaBlocked(t *testing.T) {
	if err := RunNewEnemy(44); err != nil {
		t.Fatal(err)
	}
}

func TestNewEnemyExploreSeeds(t *testing.T) {
	for _, seed := range []uint64{7, 19, 44, 91} {
		if err := RunNewEnemyExplore(seed); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}
