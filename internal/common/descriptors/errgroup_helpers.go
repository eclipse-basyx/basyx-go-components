package descriptors

import "golang.org/x/sync/errgroup"

// GoAssign runs fn within the provided errgroup and assigns its successful
// result into dst. If fn returns an error, the assignment is skipped and the
// error is propagated to the group's Wait(). This helps reduce boilerplate
// around common patterns of spawning a goroutine, collecting a value, and
// handling the error.
//
// Example:
//   var out map[int64][]T
//   GoAssign(g, func() (map[int64][]T, error) { return load(... ) }, &out)
//
// The function is generic and can be used for any result type.
func GoAssign[T any](g *errgroup.Group, fn func() (T, error), dst *T) {
    g.Go(func() error {
        v, err := fn()
        if err != nil {
            return err
        }
        *dst = v
        return nil
    })
}

