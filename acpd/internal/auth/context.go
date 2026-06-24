package auth

import "context"

func contextWithActor(ctx context.Context, a *ActorRecord) context.Context {
return context.WithValue(ctx, ActorKey{}, a)
}

func ActorFromContext(ctx context.Context) *ActorRecord {
a, _ := ctx.Value(ActorKey{}).(*ActorRecord)
return a
}
