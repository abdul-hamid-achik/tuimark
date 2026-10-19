package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// The perft counts are the leaf nodes of the legal move tree, from the
// Chess Programming Wiki "Perft Results" page
// (https://www.chessprogramming.org/Perft_Results). They are the gate of the
// move generator: a mismatch is a bug in movegen.go, never a number to edit.
// The depths in perftCases run in about a second; TUIMARK_PERFT_DEEP=1 adds
// the deeper ones.
type perftCase struct {
	name  string
	fen   string
	depth int
	nodes uint64
	deep  bool
}

var perftCases = []perftCase{
	{"start", startFEN, 1, 20, false},
	{"start", startFEN, 2, 400, false},
	{"start", startFEN, 3, 8902, false},
	{"start", startFEN, 4, 197281, false},
	{"start", startFEN, 5, 4865609, true},
	{"start", startFEN, 6, 119060324, true},

	{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq -", 1, 48, false},
	{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq -", 2, 2039, false},
	{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq -", 3, 97862, false},
	{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq -", 4, 4085603, true},
	{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq -", 5, 193690690, true},

	{"position 3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 1, 14, false},
	{"position 3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 2, 191, false},
	{"position 3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 3, 2812, false},
	{"position 3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 4, 43238, false},
	{"position 3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 5, 674624, false},
	{"position 3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 6, 11030083, true},
	{"position 3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 7, 178633661, true},

	{"position 4", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", 1, 6, false},
	{"position 4", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", 2, 264, false},
	{"position 4", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", 3, 9467, false},
	{"position 4", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", 4, 422333, false},
	{"position 4", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", 5, 15833292, true},
	// The mirrored position has the same counts.
	{"position 4 mirrored", "r2q1rk1/pP1p2pp/Q4n2/bbp1p3/Np6/1B3NBn/pPPP1PPP/R3K2R b KQ - 0 1", 3, 9467, false},
	{"position 4 mirrored", "r2q1rk1/pP1p2pp/Q4n2/bbp1p3/Np6/1B3NBn/pPPP1PPP/R3K2R b KQ - 0 1", 4, 422333, false},

	{"position 5", "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8", 1, 44, false},
	{"position 5", "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8", 2, 1486, false},
	{"position 5", "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8", 3, 62379, false},
	{"position 5", "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8", 4, 2103487, true},
	{"position 5", "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8", 5, 89941194, true},

	{"position 6", "r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10", 1, 46, false},
	{"position 6", "r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10", 2, 2079, false},
	{"position 6", "r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10", 3, 89890, false},
	{"position 6", "r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10", 4, 3894594, true},
	{"position 6", "r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10", 5, 164075551, true},
}

func TestPerft(t *testing.T) {
	deep := os.Getenv("TUIMARK_PERFT_DEEP") != ""
	start := time.Now()
	for _, c := range perftCases {
		if c.deep && !deep {
			continue
		}
		p, err := ParseFEN(c.fen)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := p.Perft(c.depth)
		t.Logf("perft %-20s depth %d  want %-12d got %d", c.name, c.depth, c.nodes, got)
		if got != c.nodes {
			t.Errorf("%s depth %d: perft = %d, want %d\ndivide: %v", c.name, c.depth, got, c.nodes, p.Divide(c.depth))
		}
	}
	t.Logf("perft took %v", time.Since(start))
}

// Each corner of the rules has a one-position probe, so a perft mismatch
// has an obvious suspect.
func TestRulesProbes(t *testing.T) {
	legal := func(fen string) map[string]bool {
		p := mustFEN(t, fen)
		var buf [256]Move
		out := map[string]bool{}
		for _, m := range p.LegalMoves(buf[:0]) {
			out[m.UCI()] = true
		}
		return out
	}
	// A rook on f3 covers f1: castling kingside crosses an attacked square.
	m := legal("4k3/8/8/8/8/5r2/8/R3K2R w KQ - 0 1")
	if m["e1g1"] || !m["e1c1"] {
		t.Errorf("castling through check: e1g1=%v e1c1=%v", m["e1g1"], m["e1c1"])
	}
	// Out of check is no castling, and neither is into it.
	m = legal("4k3/8/8/8/8/4r3/8/R3K2R w KQ - 0 1")
	if m["e1g1"] || m["e1c1"] {
		t.Error("castled out of check")
	}
	m = legal("4k3/8/8/8/8/6r1/8/R3K2R w KQ - 0 1")
	if m["e1g1"] || !m["e1c1"] {
		t.Errorf("castling into check: e1g1=%v e1c1=%v", m["e1g1"], m["e1c1"])
	}
	// The queenside rook may be attacked (only the king's path counts) but
	// the b-file square must be empty.
	m = legal("4k3/8/8/8/8/1r6/8/R3K2R w KQ - 0 1")
	if !m["e1c1"] {
		t.Error("an attacked b1 forbids castling queenside")
	}
	m = legal("4k3/8/8/8/8/8/8/RN2K2R w KQ - 0 1")
	if m["e1c1"] {
		t.Error("castled through a knight")
	}
	// Mate and stalemate leave nothing to play.
	for _, fen := range []string{"7k/5Q2/6K1/8/8/8/8/8 b - - 0 1", "R5k1/5ppp/8/8/8/8/8/6K1 b - - 0 1"} {
		if m := legal(fen); len(m) != 0 {
			t.Errorf("%s has moves %v", fen, m)
		}
	}
	// A pinned piece may only move along the pin.
	m = legal("4k3/4r3/8/8/8/8/4N3/4K3 w - - 0 1")
	for mv := range m {
		if strings.HasPrefix(mv, "e2") {
			t.Errorf("the pinned knight moves: %s", mv)
		}
	}
	// A pawn promotes four ways and a double push needs both squares free.
	m = legal("4k3/P7/8/8/8/8/8/4K3 w - - 0 1")
	for _, mv := range []string{"a7a8q", "a7a8r", "a7a8b", "a7a8n"} {
		if !m[mv] {
			t.Errorf("no %s", mv)
		}
	}
	m = legal("4k3/8/8/8/8/p7/P7/4K3 w - - 0 1")
	if m["a2a3"] || m["a2a4"] {
		t.Error("a blocked pawn moves")
	}
	m = legal("4k3/8/8/8/8/8/P7/4K3 w - - 0 1")
	if !m["a2a3"] || !m["a2a4"] {
		t.Error("a pawn on its start square cannot push one or two")
	}
}

func TestEnPassantPinnedAlongTheRank(t *testing.T) {
	// The white pawn on e5 may not take d6 en passant: both pawns leave the
	// fifth rank and the rook on h5 would check the king on a5.
	p, err := ParseFEN("8/8/8/K2pP2r/8/8/8/4k3 w - d6 0 1")
	if err != nil {
		t.Fatal(err)
	}
	var buf [256]Move
	for _, m := range p.LegalMoves(buf[:0]) {
		if m.UCI() == "e5d6" {
			t.Fatal("e5d6 en passant is legal, but it exposes the king")
		}
	}
	// Without the rook it is legal and removes the pawn.
	p, _ = ParseFEN("8/8/8/3pP2k/8/8/8/4K3 w - d6 0 1")
	np := p.Make(Move{From: 36, To: 43})
	if np.B[35] != 0 || np.B[43] != mk(White, Pawn) {
		t.Errorf("en passant left\n%s", np.String())
	}
}

func TestFENRoundTrip(t *testing.T) {
	for _, fen := range []string{
		startFEN,
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
		"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1",
		"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 12 40",
	} {
		p, err := ParseFEN(fen)
		if err != nil {
			t.Fatalf("%s: %v", fen, err)
		}
		if got := p.FEN(); got != fen {
			t.Errorf("FEN round trip\n got %s\nwant %s", got, fen)
		}
	}
	// The clocks may be left out; unusable castling rights are dropped.
	p, err := ParseFEN("4k3/8/8/8/8/8/8/4K3 w KQkq -")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.FEN(); got != "4k3/8/8/8/8/8/8/4K3 w - - 0 1" {
		t.Errorf("FEN = %s", got)
	}
}

func TestFENErrors(t *testing.T) {
	for fen, want := range map[string]string{
		"":                                              "at least",
		"8/8/8/8/8/8/8 w - - 0 1":                       "want 8",
		"4k3/8/8/8/8/8/8/8 w - - 0 1":                   "one king",
		"4k3/8/8/8/8/8/8/4K3 x - - 0 1":                 "w or b",
		"4k3/8/8/8/8/8/8/4K3 w - e4 0 1":                "wrong rank",
		"4k3/P7/8/8/8/8/8/4K3 w - - 0 1":                "", // legal: a pawn on a7
		"P3k3/8/8/8/8/8/8/4K3 w - - 0 1":                "pawn on rank 8",
		"4k3/8/8/8/8/8/8/4K3 w - - x 1":                 "halfmove",
		"4k3/8/8/8/8/8/8/4K3 w - - 0 0":                 "fullmove",
		"4k3/8/8/8/8/8/4r3/4K3 b - - 0 1":               "in check",
		"4k3/8/8/8/8/8/8/4K3Z w - - 0 1":                "unknown piece",
		"4k3/8/8/8/8/8/8/4K4 w - - 0 1":                 "files",
		"4k3/8/8/8/8/8/8/4K3 w KQkq - 0 1 extra":        "fields",
		"4k3/8/8/8/8/8/8/4K3 w M - 0 1":                 "castling",
		"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w": "",
	} {
		_, err := ParseFEN(fen)
		if want == "" {
			if err != nil {
				t.Errorf("%q: unexpected error %v", fen, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error = %v, want one with %q", fen, err, want)
		}
	}
}

func parseOrFatal(t *testing.T, p *Position, s string) Move {
	t.Helper()
	m, err := ParseMove(p, s)
	if err != nil {
		t.Fatalf("ParseMove(%q) in %s: %v", s, p.FEN(), err)
	}
	return m
}

func TestSANGenerationDisambiguation(t *testing.T) {
	cases := []struct {
		fen, uci, san string
	}{
		// Two knights reach d2: by file.
		{"4k3/8/8/8/8/8/8/1N2KN2 w - - 0 1", "b1d2", "Nbd2"},
		// Two rooks on the same file: by rank.
		{"4k3/8/8/R7/8/8/8/R3K3 w - - 0 1", "a1a3", "R1a3"},
		// Three queens: the full square.
		{"6k1/8/8/8/8/Q7/8/Q1Q1K3 w - - 0 1", "a1c3", "Qa1c3"},
		// A capture.
		{"4k3/8/8/8/8/8/4q3/4K2R w K - 0 1", "e1e2", "Kxe2"},
		{"4k3/8/8/3p4/4P3/8/8/4K3 w - - 0 1", "e4d5", "exd5"},
		// Mate.
		{"6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1", "a1a8", "Ra8#"},
		// Castling.
		{"r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "e1g1", "O-O"},
		{"r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "e1c1", "O-O-O"},
		// Promotion, plain and with capture and check.
		{"4k3/P7/8/8/8/8/8/4K3 w - - 0 1", "a7a8q", "a8=Q+"},
		{"1n2k3/P7/8/8/8/8/8/4K3 w - - 0 1", "a7b8n", "axb8=N"},
		// En passant.
		{"4k3/8/8/3pP3/8/8/8/4K3 w - d6 0 1", "e5d6", "exd6"},
	}
	for _, c := range cases {
		p, err := ParseFEN(c.fen)
		if err != nil {
			t.Fatalf("%s: %v", c.fen, err)
		}
		from, _ := parseSquare(c.uci[0:2])
		to, _ := parseSquare(c.uci[2:4])
		m := Move{From: uint8(from), To: uint8(to)}
		if len(c.uci) == 5 {
			m.Promo = map[byte]PieceType{'q': Queen, 'r': Rook, 'b': Bishop, 'n': Knight}[c.uci[4]]
		}
		if !p.IsLegal(m) {
			t.Errorf("%s: %s is not legal", c.fen, c.uci)
			continue
		}
		if got := SAN(&p, m); got != c.san {
			t.Errorf("%s %s: SAN = %q, want %q", c.fen, c.uci, got, c.san)
		}
		// And the text reads back to the same move.
		if back, err := ParseMove(&p, c.san); err != nil || back != m {
			t.Errorf("%s: ParseMove(%q) = %v, %v, want %v", c.fen, c.san, back, err, m)
		}
	}
}

func TestParseMoveLenient(t *testing.T) {
	p := StartPosition()
	for _, in := range []string{"e4", "e2e4", "E4", " e4 ", "e4!", "e4?!"} {
		m := parseOrFatal(t, &p, in)
		if m.UCI() != "e2e4" {
			t.Errorf("%q = %s", in, m.UCI())
		}
	}
	for _, in := range []string{"Nf3", "nf3", "Ng1f3", "g1f3", "Ng1-f3", "N1f3"} {
		if m := parseOrFatal(t, &p, in); m.UCI() != "g1f3" {
			t.Errorf("%q = %s", in, m.UCI())
		}
	}

	// Captures with and without x, castling spellings, promotions.
	q, _ := ParseFEN("r3k2r/1P6/8/3p4/4P3/8/8/R3K2R w KQkq - 0 1")
	for in, want := range map[string]string{
		"exd5": "e4d5", "ed5": "e4d5", "e4xd5": "e4d5", "e4d5": "e4d5",
		"O-O": "e1g1", "0-0": "e1g1", "o-o": "e1g1", "O-O+": "e1g1", "e1g1": "e1g1",
		"O-O-O": "e1c1", "0-0-0": "e1c1",
		"b8=Q": "b7b8q", "b8Q": "b7b8q", "b8=q": "b7b8q", "b7b8q": "b7b8q", "b8=N": "b7b8n", "b8=R+": "b7b8r",
		"bxa8=Q": "", "Rxa8": "",
	} {
		m, err := ParseMove(&q, in)
		if want == "" {
			continue
		}
		if err != nil || m.UCI() != want {
			t.Errorf("%q = %v, %v, want %s", in, m, err, want)
		}
	}
	// A promotion without its piece asks for it.
	m, err := ParseMove(&q, "b8")
	if err != ErrNeedPromotion || m.UCI() != "b7b8" {
		t.Errorf("b8 = %v, %v, want ErrNeedPromotion with b7b8", m, err)
	}
	if _, err := ParseMove(&q, "b7b8"); err != ErrNeedPromotion {
		t.Errorf("b7b8 error = %v, want ErrNeedPromotion", err)
	}
	// Errors say what is wrong.
	for in, want := range map[string]string{"e5": "illegal", "": "no move", "zz": "illegal", "Ke2": "illegal"} {
		_, err := ParseMove(&p, in)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error = %v, want one with %q", in, err, want)
		}
	}
	// Two knights can reach d2: the bare form is ambiguous and says which.
	n, _ := ParseFEN("4k3/8/8/8/8/8/8/1N2KN2 w - - 0 1")
	if _, err := ParseMove(&n, "Nd2"); err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "Nbd2") {
		t.Errorf("Nd2 error = %v, want an ambiguity naming Nbd2", err)
	}
	if m := parseOrFatal(t, &n, "Nfd2"); m.UCI() != "f1d2" {
		t.Errorf("Nfd2 = %s", m.UCI())
	}
}

// Morphy's Opera Game goes through the SAN parser, every move legal; the SAN
// writer gives each move's text back, and the end is checkmate.
func TestOperaGame(t *testing.T) {
	moves := operaMoves()
	if len(moves) != 33 {
		t.Fatalf("%d moves in the table, want 33", len(moves))
	}
	g := NewGame(StartPosition())
	for i, san := range moves {
		m, err := ParseMove(g.Cur(), san)
		if err != nil {
			t.Fatalf("move %d %s: %v", i+1, san, err)
		}
		if got := SAN(g.Cur(), m); got != san {
			t.Errorf("move %d: SAN writes %q, the game has %q", i+1, got, san)
		}
		if err := g.Push(m, [2]int64{}); err != nil {
			t.Fatalf("move %d %s: %v", i+1, san, err)
		}
	}
	p := g.Cur()
	if !p.InCheck() || p.HasLegalMove() {
		t.Errorf("the Opera Game does not end in checkmate\n%s", p.String())
	}
	if g.Result != "1-0" || g.Reason != "checkmate" {
		t.Errorf("result = %s %s, want 1-0 checkmate", g.Result, g.Reason)
	}
}

func TestPGNRoundTrip(t *testing.T) {
	g := NewGame(StartPosition())
	for _, san := range operaMoves() {
		m := parseOrFatal(t, g.Cur(), san)
		g.push(m, [2]int64{})
	}
	g.Tags["White"], g.Tags["Black"] = "Paul Morphy", "Duke Karl / Count Isouard"
	g.Tags["Event"], g.Tags["Site"], g.Tags["Date"] = "Opera House", "Paris FRA", "1858.??.??"
	text := g.PGN()
	for _, want := range []string{
		`[Event "Opera House"]`, `[Site "Paris FRA"]`, `[Date "1858.??.??"]`, `[Round "-"]`,
		`[White "Paul Morphy"]`, `[Black "Duke Karl / Count Isouard"]`, `[Result "1-0"]`,
		"1. e4 e5 2. Nf3 d6", "17. Rd8# 1-0",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("PGN lacks %q:\n%s", want, text)
		}
	}
	for i, l := range strings.Split(text, "\n") {
		if len(l) > 80 {
			t.Errorf("line %d is %d columns wide", i+1, len(l))
		}
	}
	back, err := ParsePGN(text)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(back.SAN, " ") != strings.Join(g.SAN, " ") || back.Result != "1-0" {
		t.Errorf("the PGN reads back as %v %s", back.SAN, back.Result)
	}
	if back.Tags["White"] != "Paul Morphy" {
		t.Errorf("tags = %v", back.Tags)
	}
}

func TestPGNImportSkipsNoise(t *testing.T) {
	text := `% an escape line
[Event "Noise"]
[White "A \"quoted\" name"]
[Black "B"]
[Result "1/2-1/2"]

1. e4 {best by test} e5 $1 2. Nf3 (2. f4 exf4 (2... d5) 3. Nf3) 2... Nc6 ; to end of line
3. Bb5 a6 {nested (not a variation)} 4.Ba4 Nf6 1/2-1/2

[Event "Second"]

1. d4 *
`
	g, err := ParsePGN(text)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(g.SAN, " "); got != "e4 e5 Nf3 Nc6 Bb5 a6 Ba4 Nf6" {
		t.Errorf("moves = %q", got)
	}
	if g.Tags["White"] != `A "quoted" name` {
		t.Errorf("White = %q", g.Tags["White"])
	}
	if g.Result != "1/2-1/2" {
		t.Errorf("result = %q", g.Result)
	}
	// A black-to-move start from a FEN tag numbers its first move 1...
	fenGame := NewGame(mustFEN(t, "4k3/8/8/8/8/8/4P3/4K3 b - - 0 7"))
	fenGame.push(parseOrFatal(t, fenGame.Cur(), "Kd7"), [2]int64{})
	out := fenGame.PGN()
	if !strings.Contains(out, `[FEN "4k3/8/8/8/8/8/4P3/4K3 b - - 0 7"]`) || !strings.Contains(out, "7... Kd7") {
		t.Errorf("FEN game PGN:\n%s", out)
	}
	back, err := ParsePGN(out)
	if err != nil || len(back.Moves) != 1 {
		t.Errorf("FEN game reads back as %v, %v", back, err)
	}
}

func TestPGNImportErrors(t *testing.T) {
	for in, want := range map[string]string{
		"1. e4 e5 2. Ke3":         "move 3",
		"1. e4 {never closed":     "comment",
		"1. e4 (2. d4":            "variation",
		"[Event Noise]\n\n1. e4":  "bad tag",
		"[FEN \"nonsense\"]\n\n*": "FEN",
	} {
		_, err := ParsePGN(in)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error = %v, want one with %q", in, err, want)
		}
	}
}

func mustFEN(t *testing.T, s string) Position {
	t.Helper()
	p, err := ParseFEN(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func play(t *testing.T, g *Game, sans ...string) {
	t.Helper()
	for _, s := range sans {
		m := parseOrFatal(t, g.Cur(), s)
		if err := g.Push(m, [2]int64{}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGameResults(t *testing.T) {
	// Fool's mate.
	g := NewGame(StartPosition())
	play(t, g, "f3", "e5", "g4", "Qh4#")
	if g.Result != "0-1" || g.Reason != "checkmate" {
		t.Errorf("fool's mate: %s %s", g.Result, g.Reason)
	}
	// Stalemate.
	g = NewGame(mustFEN(t, "7k/8/6K1/8/8/8/8/5Q2 w - - 0 1"))
	play(t, g, "Qf7")
	if g.Result != "1/2-1/2" || g.Reason != "stalemate" {
		t.Errorf("stalemate: %s %s", g.Result, g.Reason)
	}
	// Insufficient material, after the last pawn falls.
	for _, fen := range []string{
		"4k3/8/8/8/8/8/8/4K3 w - - 0 1",
		"4k3/8/8/8/8/8/8/3NK3 w - - 0 1",
		"4k3/8/8/8/8/8/8/3BK3 b - - 0 1",
		"2b1k3/8/8/8/8/8/8/3BK3 w - - 0 1", // bishops on d1 and c8, both on light squares
	} {
		p := mustFEN(t, fen)
		if !p.InsufficientMaterial() {
			t.Errorf("%s should be insufficient material", fen)
		}
	}
	for _, fen := range []string{
		"4k3/8/8/8/8/8/P7/4K3 w - - 0 1",
		"4k3/8/8/8/8/8/8/2NNK3 w - - 0 1",
		"4k3/8/8/8/8/8/8/R3K3 w - - 0 1",
		"4kb2/8/8/8/8/8/8/3BK3 w - - 0 1", // d1 is light, f8 is dark
		"4k3/8/8/8/8/8/8/2BBK3 w - - 0 1", // two bishops, c1 dark and d1 light
	} {
		p := mustFEN(t, fen)
		if p.InsufficientMaterial() {
			t.Errorf("%s is not insufficient material", fen)
		}
	}
	g = NewGame(mustFEN(t, "4k3/8/8/8/8/8/3p4/4K3 w - - 0 1"))
	play(t, g, "Kxd2")
	if g.Result != "1/2-1/2" || g.Reason != "insufficient material" {
		t.Errorf("K v K: %s %s", g.Result, g.Reason)
	}
}

func TestRepetitionAndFiftyMoveClaims(t *testing.T) {
	g := NewGame(StartPosition())
	if g.ClaimableDraw() != "" {
		t.Fatal("a draw is claimable at the start")
	}
	shuffle := []string{"Nf3", "Nf6", "Ng1", "Ng8"}
	play(t, g, shuffle...)
	if g.Repetitions() != 2 || g.ClaimableDraw() != "" {
		t.Fatalf("after one shuffle: %d repetitions, claim %q", g.Repetitions(), g.ClaimableDraw())
	}
	play(t, g, shuffle...)
	if g.Repetitions() != 3 || g.ClaimableDraw() != "threefold repetition" {
		t.Fatalf("after two shuffles: %d repetitions, claim %q", g.Repetitions(), g.ClaimableDraw())
	}
	if g.Over() {
		t.Fatal("threefold repetition ends the game by itself; it is only claimable")
	}
	if err := g.ClaimDraw(); err != nil || g.Result != "1/2-1/2" || g.Reason != "threefold repetition" {
		t.Errorf("claim: %v %s %s", err, g.Result, g.Reason)
	}
	// Undo takes the claim back with the move.
	g.Undo()
	if g.Over() {
		t.Error("undo left the claimed result")
	}
	// Five times is automatic.
	play(t, g, "Ng8")
	for i := 0; i < 2; i++ {
		play(t, g, shuffle...)
	}
	if g.Result != "1/2-1/2" || g.Reason != "fivefold repetition" {
		t.Errorf("fivefold: %s %s (%d repetitions)", g.Result, g.Reason, g.Repetitions())
	}

	// The fifty-move rule: claimable from a clock of 100, automatic at 150.
	f := NewGame(mustFEN(t, "4k3/8/8/8/8/8/4P3/R3K3 w - - 98 80"))
	play(t, f, "Ra2")
	if f.ClaimableDraw() != "" {
		t.Error("claimable at a clock of 99")
	}
	play(t, f, "Kd8")
	if f.ClaimableDraw() != "fifty-move rule" {
		t.Errorf("claim at 100 = %q", f.ClaimableDraw())
	}
	if err := f.ClaimDraw(); err != nil || f.Reason != "fifty-move rule" {
		t.Errorf("claim: %v %s", err, f.Reason)
	}
	if err := NewGame(StartPosition()).ClaimDraw(); err == nil {
		t.Error("a claim at the start worked")
	}
	h := NewGame(mustFEN(t, "4k3/8/8/8/8/8/4P3/R3K3 w - - 149 90"))
	play(t, h, "Ra3")
	if h.Result != "1/2-1/2" || h.Reason != "seventy-five-move rule" {
		t.Errorf("75 moves: %s %s", h.Result, h.Reason)
	}
}

// startKey is the key of the starting position under the fixed Zobrist seed.
const startKey = 0x6d5f1b50ccbee8cc

func TestZobristKey(t *testing.T) {
	// Transposition: the same position by two move orders has one key.
	a, b := NewGame(StartPosition()), NewGame(StartPosition())
	play(t, a, "Nf3", "Nf6", "Nc3", "Nc6")
	play(t, b, "Nc3", "Nc6", "Nf3", "Nf6")
	if a.Keys[4] != b.Keys[4] {
		t.Error("a transposition has a different key")
	}
	// A different side to move, castling right, or en passant right differs.
	base := mustFEN(t, "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1")
	noRight := mustFEN(t, "r3k2r/8/8/8/8/8/8/R3K2R w KQk - 0 1")
	black := mustFEN(t, "r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1")
	if base.Key() == noRight.Key() || base.Key() == black.Key() {
		t.Error("keys ignore the castling rights or the side")
	}
	// An en passant square counts only when a capture exists.
	withEP := mustFEN(t, "4k3/8/8/3pP3/8/8/8/4K3 w - d6 0 1")
	withoutEP := mustFEN(t, "4k3/8/8/3pP3/8/8/8/4K3 w - - 0 1")
	if withEP.Key() == withoutEP.Key() {
		t.Error("a legal en passant capture does not change the key")
	}
	noCapture := mustFEN(t, "4k3/8/8/3p4/8/8/4P3/4K3 w - d6 0 1")
	noCaptureNoEP := mustFEN(t, "4k3/8/8/3p4/8/8/4P3/4K3 w - - 0 1")
	if noCapture.Key() != noCaptureNoEP.Key() {
		t.Error("an en passant square nobody can use changes the key")
	}
	// The keys are fixed: they do not depend on the run.
	start := StartPosition()
	if k := start.Key(); k != startKey {
		t.Errorf("the start key is %#x, want %#x", k, uint64(startKey))
	}
}

func TestUndoRedo(t *testing.T) {
	g := NewGame(StartPosition())
	play(t, g, "e4", "e5", "Nf3")
	if !g.Undo() || !g.Undo() || g.Ply() != 1 {
		t.Fatalf("ply = %d after two undos", g.Ply())
	}
	if !g.Redoable() {
		t.Fatal("nothing to redo")
	}
	g.Replay()
	if g.Ply() != 2 || g.SAN[1] != "e5" {
		t.Errorf("redo: ply %d, %v", g.Ply(), g.SAN)
	}
	// A new move clears the redo stack.
	play(t, g, "Nc3")
	if g.Redoable() {
		t.Error("a new move left the redo stack")
	}
	for g.Undo() {
	}
	if g.Undo() || g.Ply() != 0 || g.Cur().FEN() != startFEN {
		t.Errorf("undo to the start: ply %d %s", g.Ply(), g.Cur().FEN())
	}
}
