package main

import (
	"context"
	"io"

	"github.com/codenotary/immudb/embedded/appendable"
	"github.com/codenotary/immudb/embedded/store"
	"gitlab.com/tozd/go/errors"
)

var _ Engine = (*Immudb)(nil)

type Immudb struct {
	db *store.ImmuStore
}

func (e *Immudb) Close() errors.E {
	return errors.WithStack(e.db.Close())
}

func (e *Immudb) Sync() errors.E {
	return errors.WithStack(e.db.Sync())
}

func (e *Immudb) Get(key []byte) (io.ReadSeekCloser, errors.E) {
	tx, err := e.db.NewTx(context.Background(), store.DefaultTxOptions().WithMode(store.ReadOnlyTx))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ref, err := tx.Get(context.Background(), key)
	if err != nil {
		return nil, errors.Join(err, tx.Cancel())
	}
	value, err := ref.Resolve()
	if err != nil {
		return nil, errors.Join(err, tx.Cancel())
	}
	return newReadSeekCloser(value, func() error {
		return errors.WithStack(tx.Cancel())
	}), nil
}

func (e *Immudb) Init(app *App) errors.E {
	// We set the max value to 6 GB so that we can test values larger than 2 GB.
	maxValueLen := 6 * 1024 * 1024 * 1024
	if !isEmpty(app.Data) {
		return errors.New("data directory is not empty")
	}
	opts := store.DefaultOptions()
	// To be able to compare between engines, we make all of them sync after every write.
	// This lowers throughput, but it makes relative differences between engines clearer.
	opts = opts.WithSyncFrequency(0)
	opts = opts.WithCompressionFormat(appendable.NoCompression)
	opts = opts.WithMaxValueLen(maxValueLen)
	opts = opts.WithLogger(loggerWrapper{app.Logger})
	db, err := store.Open(app.Data, opts)
	if err != nil {
		return errors.WithStack(err)
	}
	e.db = db
	return nil
}

func (*Immudb) Name() string {
	return "Immudb"
}

func (e *Immudb) Put(key []byte, value []byte) (errE errors.E) {
	// We want read-write tx to evaluate such transactions even if we are just writing here.
	tx, err := e.db.NewTx(context.Background(), store.DefaultTxOptions().WithMode(store.ReadWriteTx))
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		err := tx.Cancel()
		if errors.Is(err, store.ErrAlreadyClosed) {
			err = nil
		}
		errE = errors.Join(errE, err)
	}()

	err = tx.Set(key, nil, value)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = tx.Commit(context.Background())
	return errors.WithStack(err)
}
