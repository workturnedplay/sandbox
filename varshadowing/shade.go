package main

import "fmt"

func main() {
    fmt.Println("=== same-scope := ===")
    ret:=sameScope2()
		fmt.Println("returned: ",ret)

    fmt.Println("\n=== nested-scope := (shadowing) ===")
    nestedScope()
}

func sameScope() {
    var x int = 10
    fmt.Printf("before: x addr=%p val=%d\n", &x, x)

    // x already exists in THIS scope.
    // Only err is new.
    x, err := returnsTwo()
    _ = err

    fmt.Printf("after:  x addr=%p val=%d\n", &x, x)
}

func sameScope2() (x int) {
    x = 10
    fmt.Printf("before: x addr=%p val=%d\n", &x, x)

    // x already exists in THIS scope.
    // Only err is new.
    x, err := returnsTwo()
    _ = err

    fmt.Printf("after:  x addr=%p val=%d\n", &x, x)
		return
}

func sameScope3() (x int) {
    //x = 10
    //fmt.Printf("before: x addr=%p val=%d\n", &x, x)

    // x already exists in THIS scope.
    // Only err is new.
    x, _ := returnsTwo()
    //_ = err

    fmt.Printf("after:  x addr=%p val=%d\n", &x, x)
		return
}

func nestedScope() {
    var x int = 10
    fmt.Printf("outer before: x addr=%p val=%d\n", &x, x)

    if true {
        // NEW scope → this x SHADOWS the outer one
        x, err := returnsTwo()
        _ = err
        fmt.Printf("inner:        x addr=%p val=%d\n", &x, x)
    }

    fmt.Printf("outer after:  x addr=%p val=%d\n", &x, x)
}

func returnsTwo() (int, error) {
    return 42, nil
}