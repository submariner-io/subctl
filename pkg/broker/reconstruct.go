package broker

import (
	"context"
	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/rbac"
	"github.com/submariner-io/subctl/pkg/cluster"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ReconstructData(
	brokerCluster, submCluster *cluster.Info, namespace string, status reporter.Interface,
	ipsecSubmFile string) error {
	status.Start("Retrieving data to reconstruct broker-info.subm")
    defer status.End()

	data := &Info{}
	var err error

	data.BrokerURL = brokerCluster.RestConfig.Host + brokerCluster.RestConfig.APIPath

	data.ClientToken, err = rbac.GetClientTokenSecret(
		context.TODO(), brokerCluster.ClientProducer.ForKubernetes(), namespace,
		constants.SubmarinerBrokerAdminSA,
	)
	if err != nil {
		return status.Error(err, "error getting broker client secret")
	}

	if brokerCluster.Broker != nil {
		data.Components = brokerCluster.Broker.Spec.Components
		data.ServiceDiscovery = data.IsServiceDiscoveryEnabled()
		data.CustomDomains = brokerCluster.Broker.Spec.DefaultCustomDomains
	} else if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return status.Error(err, "error retrieving Broker")
	}

	if submCluster.Submariner != nil {
		status.Success("Retrieving IPSec PSK secret from Submariner found on cluster %s", submCluster.Name)
		data.IPSecPSK = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: ipsecPSKSecretName,
			},
			Data: map[string][]byte{"psk": []byte(submCluster.Submariner.Spec.CeIPSecPSK)},
		}
	} else if ipsecSubmFile != "" {
		status.Success("Retrieving IPSec PSK from the file %s", ipsecSubmFile)
		ipsecData, err := ReadInfoFromFile(ipsecSubmFile)
		if err != nil {
			return errors.Wrapf(err, "error importing IPsec PSK secret from file %q", ipsecSubmFile)
		}

		data.IPSecPSK = ipsecData.IPSecPSK
	} else {
		status.Success("Generating new IPSec PSK secret")
		data.IPSecPSK, err = newIPSECPSKSecret()
		if err != nil {
			return status.Error(err, "error generating new IPSec PSK secret")
		}
	}

	status.Success("Successfully retrieved the data. Writing it to broker-info.subm")

	err = data.writeToFile("broker-info.subm")

	return status.Error(err, "error reconstructing broker-info.subm")
}
