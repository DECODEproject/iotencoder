package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
	datastore "github.com/thingful/twirp-datastore-go"
)

type Datastore struct {
	mock.Mock
}

func (d *Datastore) WriteData(ctx context.Context, req *datastore.WriteRequest) (*datastore.WriteResponse, error) {
	args := d.Called(ctx, req)
	return args.Get(0).(*datastore.WriteResponse), args.Error(1)
}

func (d *Datastore) ReadData(ctx context.Context, req *datastore.ReadRequest) (*datastore.ReadResponse, error) {
	args := d.Called(ctx, req)
	return args.Get(0).(*datastore.ReadResponse), args.Error(1)
}

func (d *Datastore) DeleteData(ctx context.Context, req *datastore.DeleteRequest) (*datastore.DeleteResponse, error) {
	args := d.Called(ctx, req)
	return args.Get(0).(*datastore.DeleteResponse), args.Error(1)
}
