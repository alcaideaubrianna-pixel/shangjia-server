#!/usr/bin/env python3
"""Build/run actual pure replay guard with incident fixtures; never go test."""
import pathlib
import subprocess
import tempfile

root = pathlib.Path(__file__).resolve().parents[2]
source = root / "addons/youban_publish/logic/sys/collect_replay_completeness.go"
output = pathlib.Path("/tmp/youban-collect-replay-check")
with tempfile.TemporaryDirectory(prefix="youban-replay-check-") as directory:
    directory = pathlib.Path(directory)
    production = directory / "guard.go"
    production.write_text(source.read_text().replace("package sys", "package main", 1))
    main = directory / "main.go"
    main.write_text("""package main
import ("fmt"; "reflect")
func main() {
    cases := []struct{name string; facts, candidates, want []int64}{
        {"T13960 receiver split", []int64{17632,17633,17634,17635,17636}, []int64{17632,17633}, []int64{17634,17635,17636}},
        {"complete restored group", []int64{17632,17633,17634,17635,17636}, []int64{17636,17634,17632,17635,17633}, []int64{}},
        {"duplicate deliveries", []int64{17636,17636,17634}, []int64{17634,17634}, []int64{17636}},
        {"empty target source", []int64{17636}, nil, []int64{17636}},
        {"no retained facts", nil, []int64{17632}, []int64{}},
    }
    for _, c := range cases {
        got := collectReplayMissingMessageIDs(c.facts,c.candidates)
        if !reflect.DeepEqual(got,c.want) { panic(fmt.Sprintf("%s: got %v want %v",c.name,got,c.want)) }
        fmt.Println("PASS",c.name)
    }
}
""")
    subprocess.run(["go", "build", "-o", str(output), str(production), str(main)], cwd=root, check=True)
subprocess.run([str(output)], check=True)
