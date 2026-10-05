package gpu

import "testing"

func TestParse(t *testing.T) {
	out := "NVIDIA GeForce RTX 3070 Ti, 591.59, 2, 0, 0, 1129, 8192, 50, 78.75, 310.00, 50, 1860, 0, 0, 0\r\n" +
		"Tesla T4, 550.00, 10, 20, 30, 100, 15360, 40, [N/A], 70.00, [N/A], 585, 1, 120, 900\r\n"
	list, err := parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "NVIDIA GeForce RTX 3070 Ti" || list[0].MemoryTotal != 8192 || list[0].Power != 78.75 {
		t.Fatalf("first = %+v", list[0])
	}
	if list[1].Power != -1 || list[1].Fan != -1 || list[1].EncSessions != 1 || list[1].EncFPS != 120 {
		t.Fatalf("N/A handling: %+v", list[1])
	}
	if _, err := parse("a, b\n"); err == nil {
		t.Fatal("short line should fail")
	}
}
