export GOMAXPROCS=1; go test -bench=Only*. -benchtime=60s > result/proc1
export GOMAXPROCS=2; go test -bench=Only*. -benchtime=60s > result/proc2
export GOMAXPROCS=3; go test -bench=Only*. -benchtime=60s > result/proc3
export GOMAXPROCS=4; go test -bench=Only*. -benchtime=60s > result/proc4
