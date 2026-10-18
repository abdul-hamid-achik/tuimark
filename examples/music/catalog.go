package main

// track is one entry of the library. Every title, artist, and album here
// is invented; no audio exists, so the length is only the clock a track
// runs on and bpm only sets the pulse of the fake spectrum.
type track struct {
	ID     string
	Title  string
	Artist string
	Album  string
	Secs   int
	BPM    int
}

// catalog is the whole library, in album order. The play order of next and
// previous is this order.
var catalog = []track{
	{"t01", "Glass Orchard", "Marlowe Quays", "Low Tide Atlas", 221, 96},
	{"t02", "Tessellate", "Marlowe Quays", "Low Tide Atlas", 252, 112},
	{"t03", "Salt Lanterns", "Marlowe Quays", "Low Tide Atlas", 185, 84},
	{"t04", "Harbour Static", "Marlowe Quays", "Low Tide Atlas", 302, 124},
	{"t05", "Slow Cartography", "Marlowe Quays", "Low Tide Atlas", 267, 72},

	{"t06", "Last Stop Lullaby", "Ona Verdell", "Night Bus Hymns", 208, 68},
	{"t07", "Neon Puddles", "Ona Verdell", "Night Bus Hymns", 234, 104},
	{"t08", "Fare Evasion", "Ona Verdell", "Night Bus Hymns", 167, 132},
	{"t09", "Window Seat Aurora", "Ona Verdell", "Night Bus Hymns", 276, 90},
	{"t10", "Transfer Point", "Ona Verdell", "Night Bus Hymns", 192, 118},

	{"t11", "Rust Garden", "Pale Meridian", "Ferrous Bloom", 245, 100},
	{"t12", "Copper Weather", "Pale Meridian", "Ferrous Bloom", 213, 108},
	{"t13", "Magnet Tide", "Pale Meridian", "Ferrous Bloom", 318, 76},
	{"t14", "Soft Alloy", "Pale Meridian", "Ferrous Bloom", 229, 120},

	{"t15", "Poolside Fax", "Dune Cassette", "Sunburnt Tapes", 178, 128},
	{"t16", "Cassette Summer", "Dune Cassette", "Sunburnt Tapes", 202, 116},
	{"t17", "Dial Tone Dreams", "Dune Cassette", "Sunburnt Tapes", 241, 98},
	{"t18", "Sand in the Reels", "Dune Cassette", "Sunburnt Tapes", 217, 110},
	{"t19", "Laminated Sky", "Dune Cassette", "Sunburnt Tapes", 284, 82},

	{"t20", "Polite Gravity", "Halcyon Drift", "Orbit Etiquette", 237, 94},
	{"t21", "Tea at Apogee", "Halcyon Drift", "Orbit Etiquette", 259, 86},
	{"t22", "Docking Waltz", "Halcyon Drift", "Orbit Etiquette", 341, 102},

	{"t23", "Kettle Song", "Ona Verdell", "Small Hours", 156, 74},
	{"t24", "Midnight Ferns", "Ona Verdell", "Small Hours", 198, 88},
	{"t25", "Porch Light Hymn", "Ona Verdell", "Small Hours", 224, 70},
}

// trackIndex is the position of id in the catalog, or -1.
func trackIndex(id string) int {
	for i, t := range catalog {
		if t.ID == id {
			return i
		}
	}
	return -1
}
