package github

import (
	"context"

	"github.com/AtomicWasTaken/surge/internal/model"
)

// PRClient defines the interface for interacting with PR platforms.
type PRClient interface {
	GetPR(ctx context.Context, owner, repo string, prNumber int) (*model.PR, error)
	GetDiff(ctx context.Context, owner, repo string, prNumber int) (string, error)
	GetFiles(ctx context.Context, owner, repo string, prNumber int) ([]model.FileChange, error)
	GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error)
	PostReview(ctx context.Context, owner, repo string, prNumber int, review *model.ReviewInput) error
	PostComment(ctx context.Context, owner, repo string, prNumber int, body string) error
	ListComments(ctx context.Context, owner, repo string, prNumber int) ([]*model.PRComment, error)
	DeleteComment(ctx context.Context, owner, repo string, commentID int64) error
}
