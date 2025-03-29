# IDLock - Per-ID Concurrency Limiter for Go

`IDLock` is a Go concurrency utility that provides synchronization primitives for controlling access based on a generic identifier (`T comparable`). It ensures that only one operation associated with a specific ID can proceed at any given time. Additionally, it offers an optional global concurrency limit across all IDs.

## Motivation & Use Cases

Imagine scenarios where you need to perform operations based on an ID, but concurrent operations on the *same* ID would cause issues (race conditions, data corruption), while operations on *different* IDs can safely run in parallel.

*   **Processing User Data:** Ensure only one goroutine modifies a specific user's data (`UserID`) at a time, while allowing concurrent processing for different users.
*   **Resource Management:** Limit concurrent access to a specific resource identified by a unique key (`ResourceID`).
*   **API Rate Limiting (Client-Side):** Prevent sending too many concurrent requests related to the same API key or session ID.
*   **Global Resource Capping:** Limit the total number of concurrent database connections or CPU-intensive tasks across all IDs, while still serializing access per ID.

`IDLock` provides a flexible way to manage these scenarios.

## Features

*   **Per-ID Locking:** Guarantees mutual exclusion for operations associated with the same ID.
*   **Optional Global Concurrency Limit:** Set a maximum number of operations that can hold *any* ID lock concurrently.
*   **Generic:** Works with any `comparable` type as the ID (e.g., `int`, `string`, custom structs).
*   **Multiple Acquisition Methods:**
    *   `Acquire(id)`: Blocking acquire.
    *   `AcquireContext(ctx, id)`: Acquire respecting context cancellation/deadline.
    *   `AcquireWithTimeout(id, timeout)`: Acquire with a specific timeout duration.
*   **Convenience Wrapper:** `RunWithLockTimeout(id, lockTimeout, runTimeout, fn)` acquires the lock, runs a function with its own timeout, and automatically releases the lock, handling panics.
*   **Thread-Safe:** Safe for concurrent use by multiple goroutines.
*   **Configurable Logging:** Uses a simple `logger` interface, with a default implementation writing to standard output.

## Usage

### Initialization

```go
// No global limit (unlimited concurrent operations across *different* IDs)
lock := lib.NewIDLock[string](0, nil) // Uses default logger

// Global limit of 10 concurrent operations across all IDs
// Provide a custom logger (optional)
myLogger := &MyCustomLogger{} // Must implement lib.logger interface
globalLock := lib.NewIDLock[int](10, myLogger)
```

### Basic Acquire/Release

Always use `defer lock.Release(id)` immediately after a successful `Acquire` to prevent deadlocks.

```go
id := "user-123"

lock.Acquire(id)
defer lock.Release(id)

// --- Critical section for user-123 ---
fmt.Printf("Processing data for %s\n", id)
time.Sleep(100 * time.Millisecond)
// --- End critical section ---

fmt.Printf("Finished processing %s\n", id)
```

### Using the Global Limit

If `maxParallel` was set > 0 during initialization, `Acquire` might block not only because the specific ID is locked, but also because the global limit has been reached.

```go
maxGlobal := 2
globalLock := lib.NewIDLock[int](uint(maxGlobal), nil)
var wg sync.WaitGroup

for i := 0; i < maxGlobal+1; i++ {
    wg.Add(1)
    go func(userID int) {
        defer wg.Done()
        fmt.Printf("Goroutine for user %d waiting to acquire...\n", userID)
        globalLock.Acquire(userID) // Will block if global limit (2) or this specific userID is held
        defer globalLock.Release(userID)

        fmt.Printf("User %d acquired lock. Processing...\n", userID)
        time.Sleep(500 * time.Millisecond)
        fmt.Printf("User %d finished processing.\n", userID)
    }(i)
}
wg.Wait()
// You will observe that only 'maxGlobal' goroutines run concurrently.
```

### Acquire with Context

Useful for integrating with cancellation signals or request deadlines.

```go
id := "resource-abc"
ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
defer cancel()

err := lock.AcquireContext(ctx, id)
if err != nil {
    // Lock not acquired due to timeout or cancellation
    fmt.Printf("Failed to acquire lock for %s within timeout: %v\n", id, err)
    return
}
defer lock.Release(id)

// Lock acquired, proceed with operation...
fmt.Printf("Acquired lock for %s via context\n", id)
```

### Acquire with Timeout

Similar to `AcquireContext`, but uses a `time.Duration`.

```go
id := "task-456"

if lock.AcquireWithTimeout(id, 100*time.Millisecond) {
    // Lock acquired
    defer lock.Release(id)
    fmt.Printf("Acquired lock for %s via timeout\n", id)
    // Do work...
} else {
    // Lock not acquired within the timeout
    fmt.Printf("Failed to acquire lock for %s within timeout\n", id)
}
```

### Running Code with Lock (Convenience Function)

Handles acquiring, releasing, and timeouts for both locking and execution.

```go
id := "job-789"
lockTimeout := 50 * time.Millisecond
runTimeout := 200 * time.Millisecond

myTask := func() error {
    fmt.Printf("Running task for %s...\n", id)
    time.Sleep(100 * time.Millisecond) // Simulate work
    // return errors.New("something went wrong") // Example error
    fmt.Printf("Task for %s finished.\n", id)
    return nil
}

lockAcquired, err := lock.RunWithLockTimeout(id, lockTimeout, runTimeout, myTask)

if !lockAcquired {
    fmt.Printf("Could not acquire lock for %s within %v\n", id, lockTimeout)
} else if err != nil {
    // Lock was acquired, but task failed or timed out
    if errors.Is(err, context.DeadlineExceeded) {
        fmt.Printf("Task for %s timed out after %v\n", id, runTimeout)
    } else {
        fmt.Printf("Task for %s failed: %v\n", id, err)
    }
} else {
    // Lock acquired and task completed successfully
    fmt.Printf("Task for %s completed successfully.\n", id)
}
```

## Important Considerations

*   **Pairing Acquire/Release:** It is crucial to call `Release` exactly once for every successful call to `Acquire`, `AcquireContext`, or `AcquireWithTimeout`. Failure to do so will lead to deadlocks or resource leaks. Using `defer lock.Release(id)` is the recommended pattern.
*   **`RunWithLockTimeout`:** This function handles the `Acquire`/`Release` pairing automatically, making it safer for common use cases.
*   **Error Handling:** Always check the return values (`error` from `AcquireContext`, `bool` from `AcquireWithTimeout`/`RunWithLockTimeout`, `error` from `RunWithLockTimeout`) to handle cases where the lock was not acquired.
*   **Panic Recovery:** `RunWithLockTimeout` recovers from panics within the provided function, logs the panic, and returns an error while still ensuring the lock is released.

## Logging

`IDLock` accepts an optional logger that implements the following interface:

```go
type logger interface {
    Printf(format string, args ...interface{})
}
```

If `nil` is passed to `NewIDLock`, a default logger writing to `fmt.Printf` will be used for internal warnings (e.g., attempting to release a lock that isn't held). You can provide your own logger (e.g., using `log/slog`, `logrus`, `zap`) by creating a simple wrapper that satisfies the interface.
```