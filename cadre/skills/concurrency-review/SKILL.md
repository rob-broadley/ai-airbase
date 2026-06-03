---
name: concurrency-review
description: Load before any concurrency review. Required by the concurrency-reviewer agent — covers race conditions, deadlocks, resource leaks, shared state misuse, synchronisation primitives, and cancellation propagation across JavaScript/TypeScript, Python, Java, C#, C++, Go, and Rust.
license: AGPL-3.0-or-later
---

# Concurrency Review Reference

Concurrency bugs are among the hardest to reproduce and diagnose. They appear under load, disappear under a debugger, and often produce silent data corruption rather than obvious crashes. Every section below states the language-agnostic principle first, then gives language-specific signals.

______________________________________________________________________

## Race conditions

A race condition occurs when two or more concurrent units (goroutines, threads, async tasks) access the same memory location concurrently, and at least one access is a write, without synchronisation.

**What to look for — universal signals:**

- A variable or data structure modified by one concurrent unit while another reads or modifies it, with no lock, atomic operation, or channel/queue protecting the access.
- A counter or flag modified by concurrent units without atomicity guarantees — `counter++` is not atomic in most languages even for primitive types.
- A closure or lambda capturing a variable by reference in a concurrent unit launched inside a loop — by the time the unit runs, the variable may have advanced.

**Language-specific signals:**

- **JavaScript and TypeScript:** on Node.js, the event loop is single-threaded per worker, but async interleavings still create logical races — two `async` functions read and update the same module-level state between `await` points without coordination. `worker_threads` or `SharedArrayBuffer` code introduces true shared-memory races unless `Atomics` or message passing is used correctly.
- **Python:** `threading.Thread` or `asyncio` tasks sharing mutable state without a `threading.Lock` or other coordination. The GIL protects many built-in operations but not multi-step compound operations such as read-modify-write on a dict.
- **Java:** `HashMap` modified and read from multiple threads without `ConcurrentHashMap` or synchronisation. `++` on a shared `int` field without `AtomicInteger` or `synchronized`.
- **C#:** shared `List<T>` or `Dictionary<TKey, TValue>` accessed from multiple tasks or threads without `lock`, `ConcurrentDictionary`, or `Interlocked`. `Task.Run` closures capturing mutable loop variables are a repeat offender.
- **C++:** `std::vector`, raw pointers, or shared structs accessed from multiple `std::thread` instances without `std::mutex` or `std::atomic`. Concurrent unsynchronised access to the same memory location is undefined behaviour, even when it seems to work in tests.
- **Go:** struct fields or package-level variables written from one goroutine and read from another with no mutex or atomic. Closure capturing `item` in `go func() { process(item) }()` inside a range loop. Detection: `go test -race ./...`
- **Rust:** `Arc<Mutex<T>>` or `Arc<RwLock<T>>` shared across tasks with ad-hoc interior mutability, `static mut`, or unsafe FFI access that bypasses the compiler's usual guarantees.

______________________________________________________________________

## Deadlocks

A deadlock occurs when two or more concurrent units are each waiting for the other to release a resource, resulting in all of them blocking indefinitely.

**What to look for — universal signals:**

- Lock ordering violations: two code paths acquire the same two locks in opposite orders. Under concurrent execution this produces a circular wait.
- A concurrent unit that holds a lock and then tries to acquire the same non-re-entrant lock again.
- Two concurrent units each waiting for the other to produce a value via a message/channel/queue, with no third unit to break the cycle.

**Language-specific signals:**

- **JavaScript and TypeScript:** Promise chains where A awaits B and B awaits A will hang indefinitely. Worker threads waiting on each other through blocking message protocols create traditional deadlocks even though one event loop is single-threaded.
- **Python:** `threading.Lock` acquired twice from the same thread without `threading.RLock`. `asyncio` coroutines awaiting each other in a cycle also hang permanently.
- **Java:** `synchronized` blocks on two objects acquired in opposite order from two threads. `Object.wait()` called without holding the object's monitor.
- **C#:** two `lock` statements acquired in opposite order across tasks or threads. Blocking on `.Result` or `.Wait()` while the awaited continuation needs the same context can deadlock UI or request threads.
- **C++:** `std::mutex` instances acquired in inconsistent order, or `std::condition_variable::wait` used without a predicate so a notification is missed and the thread sleeps forever.
- **Go:** `sync.Mutex` is not re-entrant — a goroutine holding a mutex that calls a function which tries to acquire the same mutex will deadlock. Unbuffered channel send with no goroutine ready to receive, or vice versa.
- **Rust:** holding a `std::sync::Mutex` guard across a call that tries to re-lock the same mutex, or awaiting while holding a Tokio mutex guard needed by another task.

______________________________________________________________________

## Resource leaks

A resource leak occurs when a concurrent unit starts but never terminates, or when a resource acquired for concurrent use is never released.

**What to look for — universal signals:**

- A concurrent unit started with no mechanism by which it can be told to stop.
- A concurrent unit that blocks waiting for a message, event, or value that will never arrive.
- A concurrent unit started in a request handler that can outlive the request, holding references to request-scoped resources.
- A completion-tracking mechanism (WaitGroup, CountDownLatch, Promise, gather) that is never signalled, causing the caller to block forever.

**Language-specific signals:**

- **JavaScript and TypeScript:** unresolved Promises held in a long-lived data structure, `setInterval` handles never cleared, or Worker threads started without a termination path.
- **Python:** `threading.Thread` with no daemon flag and no `join()` — Python waits for all non-daemon threads before exit. `asyncio.Task` created with `asyncio.create_task()` and not awaited or cancelled.
- **Java:** `ExecutorService` created but `shutdown()` or `shutdownNow()` never called — the JVM cannot exit. `Thread` started but `interrupt()` is never called and the thread has no exit condition.
- **C#:** fire-and-forget `Task.Run` work with no `CancellationToken`, `Timer` instances never disposed, or `IAsyncDisposable` resources abandoned in background services.
- **C++:** detached `std::thread` work with no ownership or shutdown path, or threads waiting on queues that are never signalled during shutdown.
- **Go:** `go func() { ... }()` with no `context.Context`, no done channel, and no cancellation signal. A goroutine reading from a channel nobody will ever close or write to. `wg.Add(1)` without a corresponding `defer wg.Done()` inside the goroutine.
- **Rust:** spawned Tokio tasks with no join handle tracking, channels whose senders are never dropped so receivers never exit, or blocking work spawned without a shutdown signal.

______________________________________________________________________

## Shared state misuse

Accessing shared state incorrectly produces data corruption that may not surface immediately.

**What to look for — universal signals:**

- Shared collections (maps, lists, arrays) read and written concurrently without synchronisation.
- A value read from shared state, acted on, and then the state written back — without holding a lock for the entire read-modify-write cycle.
- Shared state that appears to work correctly under light load but fails under concurrent access (hidden by low contention).

**Language-specific signals:**

- **JavaScript and TypeScript:** shared module-level mutable state mutated by concurrent async functions between `await` points, or Worker threads mutating shared buffers without `Atomics`.
- **Python:** shared `dict` or `list` modified from multiple threads. Individual dict operations are often GIL-protected, but compound operations such as check-then-set are not.
- **Java:** `ArrayList` or `HashMap` used in place of `CopyOnWriteArrayList` or `ConcurrentHashMap` in concurrent code.
- **C#:** `List<T>` or `Dictionary<TKey, TValue>` used where `ConcurrentBag<T>` or `ConcurrentDictionary<TKey, TValue>` is required, or a read-modify-write cycle occurs outside a `lock`.
- **C++:** `std::vector`, `std::map`, or shared raw memory modified concurrently without a lock, or `std::atomic` used for one field while adjacent non-atomic fields still race.
- **Go:** `map` read and written from multiple goroutines without a `sync.RWMutex` or `sync.Map`. Slice appended to from multiple goroutines concurrently.
- **Rust:** interior mutability (`RefCell`, `Mutex`, `RwLock`) used to share state across tasks without a clear ownership model, or `unsafe` shared state crossing thread boundaries.

______________________________________________________________________

## Synchronisation primitives

Using the wrong synchronisation primitive, or misusing the correct one, introduces bugs that are as harmful as not synchronising at all.

**What to look for — universal signals:**

- A read/write lock used where an exclusive lock was needed, or vice versa.
- A lock copied by value (most lock implementations must not be copied after first use).
- A completion barrier (WaitGroup, CountDownLatch, Promise.all) where the count or task set is incorrectly constructed.
- Using a heavyweight primitive (full mutex) where a lightweight one (atomic counter) is sufficient.

**Language-specific signals:**

- **JavaScript and TypeScript:** there is no built-in mutex for ordinary async code; if shared state must survive across `await` points, use a well-understood serialisation pattern. Shared state between Workers must use `SharedArrayBuffer` with `Atomics` for safe concurrent access.
- **Python:** `threading.Lock` vs `threading.RLock` — use `RLock` when the same thread may acquire the lock recursively. `threading.Semaphore` vs `threading.BoundedSemaphore` — prefer `BoundedSemaphore` to catch release-without-acquire bugs.
- **Java:** `synchronized` on a local variable has no effect because each caller gets its own lock. `volatile` provides visibility, not atomicity, so it does not protect a compound read-modify-write.
- **C#:** choose between `lock`, `SemaphoreSlim`, `Monitor`, and `Interlocked` deliberately. Using `lock` for simple counters or `Interlocked` for multi-step state transitions is the wrong trade-off.
- **C++:** choose between `std::mutex`, `std::shared_mutex`, `std::condition_variable`, and `std::atomic` deliberately. Copying lock-owning types, or mixing atomics with non-atomic reads of the same state, is a correctness bug.
- **Go:** `sync.Mutex` passed or embedded in a struct by value — use pointer receivers or pass by pointer. For read-heavy access, `sync.RWMutex` may be appropriate; for simple counters, `sync/atomic` is usually clearer.
- **Rust:** choose between `std::sync::Mutex`, `RwLock`, atomics, and async-aware primitives carefully. Using a blocking mutex inside async code, or holding a lock guard across `.await`, is a common mistake.

______________________________________________________________________

## Messaging and channel patterns

Message-passing concurrency has its own failure modes distinct from shared-memory concurrency.

**What to look for — universal signals:**

- A message producer that can produce faster than the consumer can consume, with no backpressure mechanism — an unbounded buffer or an eventual crash.
- A channel or queue closed or cancelled from the consumer side while the producer is still sending.
- A blocking send or receive in a code path where the counterpart may never arrive.

**Language-specific signals:**

- **JavaScript and TypeScript:** unhandled Promise rejection in a chain, missing `await` on a spawned async operation, or a Worker message protocol with no timeout or shutdown path.
- **Python:** `asyncio.Queue.join()` without matching `task_done()` calls blocks forever. Thread-safe queues need explicit sentinel values or shutdown signalling.
- **Java:** `BlockingQueue.put()` with no timeout blocks indefinitely if the queue is full. `Future.get()` with no timeout blocks indefinitely if the task never completes.
- **C#:** `Channel<T>` readers awaiting forever because writers are never completed, or `Task.WhenAll` waiting on fire-and-forget tasks that never settle.
- **C++:** `std::condition_variable::wait` without a predicate risks missed wake-ups, and producer-consumer queues with no shutdown sentinel leave consumers blocked forever.
- **Go:** closing a channel from the receiver side races with any concurrent sender and will panic at runtime. Sending to a closed channel panics. Ranging over a channel (`for v := range ch`) that is never closed blocks forever. `nil` channel in a `select` case is silently ignored and can mask bugs.
- **Rust:** Tokio or crossbeam channels where the sender side is cloned without ownership discipline, so receivers never observe closure and wait indefinitely.

______________________________________________________________________

## Distributed concurrency

### Idempotency

- Write operations invoked over a network must be idempotent or protected with an idempotency key.
- Retried writes without idempotency create duplicate side effects (double charges, duplicate records).

### Exactly-once delivery

- Broker "exactly-once" guarantees are unreliable across failure boundaries.
- The outbox pattern: write to a local outbox table in the same transaction as the domain write; a separate relay process publishes from the outbox — this is the only reliable way to achieve exactly-once semantics.
- Consumer-side deduplication using a seen-message-id table.

### Distributed locks

- Naive `SETNX`-style Redis locks have split-brain failure modes: the lock holder may have died without releasing, and a new holder is elected before the first holder's operations complete.
- Fence tokens: the lock server issues a monotonically increasing token with each lock grant; write operations carry the token; the storage layer rejects writes with stale tokens.

### Saga compensation

- Long-running distributed transactions use sagas: a sequence of local transactions each with a defined compensation step.
- Every saga step must define what to do if a later step fails — compensation steps must be idempotent.

### Bounded in-flight work

- Message consumers without a concurrency limit will spawn unbounded concurrent units under burst load.
- Apply explicit concurrency limits at the consumer level; use backpressure rather than unbounded queuing.

### Backoff and jitter on retries

- Fixed-interval retries under failure cause thundering-herd: all callers retry simultaneously after the same interval, repeating the failure.
- Exponential backoff with jitter (randomised delay) spreads retry load across the recovery window.

______________________________________________________________________

## Cancellation propagation

A concurrent unit that cannot be told to stop is a resource leak and a graceful shutdown problem.

**What to look for — universal signals:**

- A concurrent unit that does long-running work without accepting or checking a cancellation signal.
- A cancellation signal that is not propagated to sub-units or external calls spawned from the unit.
- A cancellation that is checked too infrequently — only at the start of the loop, not inside the loop body.

**Language-specific signals:**

- **JavaScript and TypeScript:** `AbortController` or `AbortSignal` not passed to `fetch` or other cancellable APIs, or timers and Worker threads not cleaned up when the surrounding request or job is cancelled.
- **Python:** `asyncio` task cancellation raises `CancelledError`; catching and suppressing it prevents cancellation from propagating. `threading.Event` used to signal threads to stop but not checked inside blocking waits.
- **Java:** `Future.cancel(true)` does not interrupt the thread unless the task checks `Thread.interrupted()`. `ExecutorService` tasks that block on I/O must use interruptible I/O or check the interrupted flag.
- **C#:** `CancellationToken` accepted at the API boundary but not passed to downstream `Task` work, I/O calls, or retry loops.
- **C++:** `std::jthread` or `std::stop_token` support ignored, or custom stop flags checked only before entering a long blocking wait.
- **Go:** `context.Context` not passed to goroutines or external calls. `ctx.Done()` not checked in long-running loops. `context.Background()` used where a child context derived from the request context was appropriate.
- **Rust:** cancellation implemented via shutdown channels or cancellation tokens, but spawned tasks do not `select!` on the shutdown signal or ignore it inside long-running loops.

______________________________________________________________________

## C\#

- `async void`: exception propagation is lost — exceptions thrown in `async void` methods escape normal task handling; prefer `Task` or `Task<T>` so callers can observe failures.
- `.Wait()` / `.Result` in synchronous contexts can deadlock when the awaited work needs to resume on the same captured context.
- Missing `ConfigureAwait(false)` in reusable library code can cause surprising context capture in callers that still use a synchronisation context.
- `lock` on `this`, a public field, or `typeof(T)` exposes the lock object to external code and invites deadlocks.

______________________________________________________________________

## C++

- `std::thread` detached without a clear lifetime owner creates shutdown leaks and hidden races. Prefer joinable ownership such as `std::jthread` where available.
- Concurrent access to shared memory without `std::mutex` or `std::atomic` is undefined behaviour, even when the code appears stable under light load.
- `std::condition_variable` waits must always use a predicate to handle spurious wake-ups correctly.
- Mixing atomics and non-atomics for the same logical state violates the memory model and hides partial races.

______________________________________________________________________

## Rust

- `Arc<Mutex<T>>` lock poisoning ignored: if a thread panics while holding a `Mutex`, subsequent `.lock()` calls return `Err`; blindly unwrapping hides the original failure mode.
- Blocking work inside async tasks: calling blocking I/O or `std::thread::sleep` inside `tokio::spawn` starves the async executor — use `tokio::task::spawn_blocking` for blocking or CPU-bound work.
- `async-trait` heap allocation: every `async fn` in a trait compiled via `async-trait` allocates a `Box<dyn Future>` — this is a hot-path concern in high-throughput code.

______________________________________________________________________

## Detection tooling

| Language                      | Tool                       | Command                                                                                                  |
| ----------------------------- | -------------------------- | -------------------------------------------------------------------------------------------------------- |
| **JavaScript and TypeScript** | ESLint promise rules       | `eslint . --rule 'promise/catch-or-return:error' --rule '@typescript-eslint/no-floating-promises:error'` |
| **Python**                    | Ruff                       | `ruff check .`                                                                                           |
| **Java**                      | SpotBugs                   | `mvn spotbugs:check` or `gradle spotbugsMain`                                                            |
| **C#**                        | Roslyn analysers           | `dotnet build`                                                                                           |
| **C++**                       | ThreadSanitizer            | build and run with `-fsanitize=thread`                                                                   |
| **Go**                        | Race detector (built-in)   | `go test -race ./...`                                                                                    |
| **Rust**                      | Clippy and compiler checks | `cargo clippy -- -W clippy::await_holding_lock`                                                          |

Race detector output (Go) and ThreadSanitizer findings (C++) are automatic Blocking findings. SpotBugs threading findings (Java) are Blocking or Recommendation depending on the specific rule.

______________________________________________________________________

## Severity model

### Blocking

Concurrency error that will cause data corruption, crashes, or deadlock under concurrent load. A race detector finding is always Blocking.

Examples: unprotected shared map write; lock ordering violation; goroutine/thread started with no exit condition in a long-lived service; channel closed from receiver side.

### Recommendation

Concurrency code that is correct under current conditions but brittle or likely to fail under different load or usage patterns.

Examples: shared read-heavy map using exclusive lock where RWMutex would suffice; missing timeout on a blocking call that is unlikely but possible to hang; cancellation signal accepted but not checked inside a long loop.

### Observation

Minor quality issue or use of a suboptimal but functionally correct primitive.

Examples: `sync.Mutex` where `sync.Once` is the idiomatic choice; `BoundedSemaphore` preferred over `Semaphore`; a concurrent unit that could be simplified to a sequential one.
