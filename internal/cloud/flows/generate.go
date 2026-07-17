//go:build ignore

package flows

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -Werror" bpf ./bpf_probe/tc.c -- -I./headers  -I/usr/include/x86_64-linux-gnu/ -I/usr/i686-linux-gnu/include/
