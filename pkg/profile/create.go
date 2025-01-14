package profile

import (
	"context"
	"fmt"
	arlonv1 "github.com/arlonproj/arlon/api/v1"
	"github.com/arlonproj/arlon/pkg/argocd"
	"github.com/arlonproj/arlon/pkg/bundle"
	"github.com/arlonproj/arlon/pkg/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"path"
)

func Create(
	config *restclient.Config,
	argocdNs string,
	arlonNs string,
	profileName string,
	repoUrl string,
	repoBasePath string,
	repoRevision string,
	bundles []string,
	desc string,
	tags string,
	overrides []arlonv1.Override,
) error {
	for _, name := range bundles {
		if !bundle.IsValidK8sName(name) {
			return fmt.Errorf("%w: %s", bundle.ErrInvalidName, name)
		}
	}
	cli, err := controller.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to get controller runtime client: %s", err)
	}
	bundlesList, err := bundle.List(config, arlonNs)
	if err != nil {
		return err
	}
	existingBundleNames := bundleListToNameSlice(bundlesList)
	if !isSubset(bundles, existingBundleNames) {
		return ErrMissingBundles
	}
	var repoPath string
	if repoUrl == "" {
		repoRevision = ""
	} else {
		repoPath = path.Join(repoBasePath, profileName)
	}
	tagList := StringListFromCommaSeparated(tags)
	p := arlonv1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      profileName,
			Namespace: arlonNs,
		},
		Spec: arlonv1.ProfileSpec{
			Description:  desc,
			Bundles:      bundles,
			Tags:         tagList,
			RepoUrl:      repoUrl,
			RepoPath:     repoPath,
			RepoRevision: repoRevision,
			Overrides:    overrides,
		},
	}
	if repoUrl != "" {
		kubeClient, err := kubernetes.NewForConfig(config)
		if err != nil {
			return fmt.Errorf("failed to get kubernetes client: %s", err)
		}
		corev1 := kubeClient.CoreV1()
		bundles, err := bundle.GetBundlesFromProfile(&p, corev1, arlonNs)
		if err != nil {
			return fmt.Errorf("failed to get bundles: %s", err)
		}
		creds, err := argocd.GetRepoCredsFromArgoCd(kubeClient, argocdNs, repoUrl)
		if err != nil {
			return fmt.Errorf("failed to get repository credentials: %s", err)
		}
		err = createInGit(creds, &p, argocdNs, bundles)
		if err != nil {
			return fmt.Errorf("failed to create dynamic profile in git: %s", err)
		}
	}
	err = cli.Create(context.Background(), &p)
	if err != nil {
		return fmt.Errorf("failed to create profile configmap: %s", err)
	}
	return nil
}
