package world

import "time"

// ResourceKind describes a gatherable node placed on the map in Tiled.
type ResourceKind struct {
	Tool   string        // the tool that strikes it
	Hits   int           // strikes needed to deplete it
	Item   string        // what it drops
	Amount int           // how many it drops when depleted
	Regrow time.Duration // real time before it comes back, online or not
	// BlocksWhenDepleted keeps the tile solid after depletion (a stump).
	BlocksWhenDepleted bool
}

// ResourceKinds lists every kind a map may use, keyed by the Tiled object type.
// Regrowth follows real time, online or not.
var ResourceKinds = map[string]ResourceKind{
	"arbre":  {Tool: "hache", Hits: 3, Item: "bois", Amount: 3, Regrow: 3 * time.Minute, BlocksWhenDepleted: true},
	"rocher": {Tool: "pioche", Hits: 4, Item: "pierre", Amount: 2, Regrow: 5 * time.Minute},
	"herbes": {Tool: "faux", Hits: 1, Item: "fibre", Amount: 1, Regrow: 90 * time.Second},
}

// ResourceNode is one live resource in a domain.
type ResourceNode struct {
	ID       int
	Kind     string
	At       Point
	Hits     int
	Depleted bool
	RegrowAt time.Time
}

func (r *ResourceNode) def() ResourceKind { return ResourceKinds[r.Kind] }

// blocks reports whether the node stops movement onto its tile.
func (r *ResourceNode) blocks() bool {
	return !r.Depleted || r.def().BlocksWhenDepleted
}
