#!/usr/bin/env bash

export GITROOT="$(git rev-parse --show-toplevel)"
source "${GITROOT}/dev/env/scripts/lib.sh"
init

if [[ "$CLUSTER_TYPE" == "minikube" ]]; then
    if ! minikube status >/dev/null; then
        minikube start --memory=5G \
            --cpus=4 \
            --apiserver-port=8443 \
            --embed-certs=true \
            --apiserver-names=minikube \
            --delete-on-failure=true
    else
        if ! kc_output=$($KUBECTL api-versions >/dev/null 2>&1); then
            die "Sanity check for contacting Kubernetes cluster failed: ${kc_output}"
        fi
    fi
fi

# Create Namespaces.
apply "${MANIFESTS_DIR}/shared"
wait_for_default_service_account "$ACSMS_NAMESPACE"

apply "${MANIFESTS_DIR}/rhacs-operator/00-namespace.yaml"
wait_for_default_service_account "$STACKROX_OPERATOR_NAMESPACE"

inject_ips() {
    local namespace="$1"
    local name="$2"

    log "Patching ServiceAccount ${namespace}/default to use Quay.io imagePullSecrets"
    $KUBECTL -n "$namespace" patch sa default -p "\"imagePullSecrets\": [{\"name\": \"${name}\" }]"
}

# TODO: use a function.
if [[ "$INHERIT_IMAGEPULLSECRETS" == "true" ]]; then
    create-imagepullsecrets-interactive
    inject_ips "$ACSMS_NAMESPACE" "quay-ips"
    inject_ips "$STACKROX_OPERATOR_NAMESPACE" "quay-ips"
fi

if [[ "$INSTALL_OPERATOR" == "true" ]]; then
    if [[ "$INSTALL_OLM" == "true" ]]; then
        # Setup OLM
        operator-sdk olm install
    fi

    if [[ "$OPERATOR_SOURCE" == "quay" ]]; then
        apply "${MANIFESTS_DIR}"/rhacs-operator/quay/01*
    fi

    if [[ "$OPERATOR_SOURCE" == "quay" && "$INHERIT_IMAGEPULLSECRETS" == "true" ]]; then
        echo "Patching ServiceAccount ${STACKROX_OPERATOR_NAMESPACE}/stackrox-operator-test-index to use imagePullSecrets"
        $KUBECTL -n "$STACKROX_OPERATOR_NAMESPACE" patch sa stackrox-operator-test-index -p '"imagePullSecrets": [{"name": "quay-ips" }]'
    fi

    if [[ "$OPERATOR_SOURCE" == "quay" ]]; then
        # Need to wait with the subscription creation until the catalog source has been updated,
        # otherwise the subscription will be in a failed state and not progress.
        # Looks like there is some race which causes the subscription to still fail right after
        # operatorhubio catalog is ready, which is why an additional delay has been added.
        $KUBECTL -n olm wait --timeout=120s --for=condition=ready pod -l olm.catalogSource=operatorhubio-catalog
        echo "Waiting for CatalogSource to include rhacs-operator..."
        while true; do
            $KUBECTL -n "$STACKROX_OPERATOR_NAMESPACE" get packagemanifests.packages.operators.coreos.com -o json |
                jq -r '.items[].metadata.name' | grep -q '^rhacs-operator$' && break
            sleep 1
        done

        echo "Waiting for CatalogSource to include bundles from operatorhubio-catalog..."
        while true; do
            $KUBECTL -n "$STACKROX_OPERATOR_NAMESPACE" get packagemanifests.packages.operators.coreos.com -o json |
                jq -r '.items[].metadata.labels.catalog' | grep -q '^operatorhubio-catalog$' && break
            sleep 1
        done
    fi

    if [[ "$OPERATOR_SOURCE" == "quay" ]]; then
        apply "${MANIFESTS_DIR}"/rhacs-operator/quay/0[23]*
    elif [[ "$OPERATOR_SOURCE" == "marketplace" ]]; then
        apply "${MANIFESTS_DIR}"/rhacs-operator/marketplace/0[23]*
    fi

    if [[ "$OPERATOR_SOURCE" == "quay" ]]; then
        echo "Waiting for SA to appear..."
        while true; do
            $KUBECTL -n "$STACKROX_OPERATOR_NAMESPACE" get serviceaccount rhacs-operator-controller-manager >/dev/null 2>&1 && break
            sleep 1
        done

        echo "Patching ServiceAccount rhacs-operator-controller-manager to use imagePullSecrets"
        if [[ "$INHERIT_IMAGEPULLSECRETS" == "true" ]]; then
            $KUBECTL -n "$STACKROX_OPERATOR_NAMESPACE" patch sa rhacs-operator-controller-manager -p '"imagePullSecrets": [{"name": "quay-ips" }]'
        fi

        sleep 2 # Wait for rhacs-operator pods to be created. Possibly the imagePullSecrets were not picked up yet, which is why we respawn them:
        $KUBECTL -n "$STACKROX_OPERATOR_NAMESPACE" delete pod -l app=rhacs-operator
    fi

    sleep 1
    $KUBECTL -n "$STACKROX_OPERATOR_NAMESPACE" wait --timeout=120s --for=condition=ready pod -l app=rhacs-operator
else
    # We will be running without RHACS operator, but at least install our CRDs.
    apply "${MANIFESTS_DIR}/crds"
fi

load_image_into_minikube() {
    local img="$1"

    if $MINIKUBE image ls | grep -q "^${img}$"; then
        true
    else
        $DOCKER pull "${img}" && $DOCKER save "${img}" | $MINIKUBE ssh --native-ssh=false docker load
    fi
}

if [[ "$CLUSTER_TYPE" == "minikube" ]]; then
    log "Preloading images into minikube..."
    # Preload images required by Central installation.
    load_image_into_minikube "${IMAGE_REGISTRY}/scanner:${SCANNER_VERSION}"
    load_image_into_minikube "${IMAGE_REGISTRY}/scanner-db:${SCANNER_VERSION}"
    load_image_into_minikube "${IMAGE_REGISTRY}/main:${CENTRAL_VERSION}"
    log "Images preloaded"
fi

log "** Bootstrapping complete **"
