package target_test

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestTarget(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Target Suite")
}

var ctx context.Context

var _ = BeforeSuite(func() {
	if os.Getenv("DEBUG") != "" {
		ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))
	}
	ctx = log.IntoContext(context.TODO(), ctrl.Log)
})
