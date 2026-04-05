// FIXED version — size is now on the heap forever
func callWithRetryRepro(who string, initialSize uint32, call func(bufPtr *byte, sizePtr *uint32) error) ([]byte, error) {
	sizePtr := new(uint32)   // ← heap allocated, never moves
	*sizePtr = initialSize

	const MAX_RETRIES = 10

	for tries := 0; tries < MAX_RETRIES; tries++ {
		fmt.Printf("!%s before6 try %d, initialSize=%d size=%d\n", who, tries, initialSize, *sizePtr)

		var buf []byte
		var ptr *byte
		if *sizePtr > 0 {
			buf = make([]byte, *sizePtr)
			ptr = &buf[0]
			fmt.Printf("!%s middle7(created buf) try %d, buf=%p ptr=%p size=%d len=%d\n",
				who, tries, buf, ptr, *sizePtr, len(buf))
		}

		fmt.Printf("!%s before7 try %d, ptr=%p &sizePtr=%p size=%d\n", who, tries, ptr, sizePtr, *sizePtr)

		err := call(ptr, sizePtr)   // ← now always heap pointer

		runtime.KeepAlive(ptr)
		runtime.KeepAlive(sizePtr)

		fmt.Printf("!%s after7 try %d, ptr=%p &sizePtr=%p size=%d\n", who, tries, ptr, sizePtr, *sizePtr)

		if err == nil {
			fmt.Printf("!%s middle7(ret ok) try %d, buf=%p len=%d size=%d\n",
				who, tries, buf, len(buf), *sizePtr)
			if uint64(*sizePtr) > uint64(len(buf)) {
				panic("impossible")
			}
			return buf, nil
		}

		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) &&
			!errors.Is(err, windows.ERROR_MORE_DATA) {
			return nil, err
		}

		if uint64(*sizePtr) <= uint64(len(buf)) {
			const increment = 1024
			if math.MaxUint32-*sizePtr < increment {
				return nil, fmt.Errorf("overflow")
			}
			*sizePtr += increment
		}
		fmt.Printf("!%s after6(end of for) try %d\n", who, tries)
	}
	return nil, fmt.Errorf("retries exceeded")
}