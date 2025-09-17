#!/bin/bash
set -e

# Validation script for provider-backblaze Crossplane v2 support
# This script validates that the provider is properly deployed with dual-scope support

echo "🔍 Validating provider-backblaze Crossplane v2 deployment..."

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed or not in PATH"
    exit 1
fi

# Check if provider is installed and healthy
echo "📦 Checking provider installation status..."
PROVIDER_STATUS=$(kubectl get provider.pkg.crossplane.io provider-backblaze -o jsonpath='{.status.conditions[?(@.type=="Healthy")].status}' 2>/dev/null || echo "NotFound")

if [ "$PROVIDER_STATUS" != "True" ]; then
    echo "❌ Provider is not installed or not healthy. Status: $PROVIDER_STATUS"
    echo "   Run: kubectl get provider.pkg.crossplane.io provider-backblaze -o yaml"
    exit 1
fi

echo "✅ Provider is installed and healthy"

# Check provider pod is running
echo "🏃 Checking provider pod status..."
POD_STATUS=$(kubectl get pods -n crossplane-system -l app.kubernetes.io/name=provider-backblaze -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "NotFound")

if [ "$POD_STATUS" != "Running" ]; then
    echo "❌ Provider pod is not running. Status: $POD_STATUS"
    echo "   Run: kubectl get pods -n crossplane-system -l app.kubernetes.io/name=provider-backblaze"
    exit 1
fi

echo "✅ Provider pod is running successfully"

# Validate CRDs are installed
echo "🎯 Validating CRD installation..."

EXPECTED_CRDS=(
    # v1 APIs (cluster-scoped - legacy support)
    "buckets.backblaze.crossplane.io"
    "users.backblaze.crossplane.io"
    "policies.backblaze.crossplane.io"
    "providerconfigs.backblaze.crossplane.io"
    "providerconfigusages.backblaze.crossplane.io"
    # v1beta1 APIs (namespaced - Crossplane v2)
    "buckets.bucket.backblaze.m.crossplane.io"
    "users.user.backblaze.m.crossplane.io"
    "policies.policy.backblaze.m.crossplane.io"
)

MISSING_CRDS=()
for crd in "${EXPECTED_CRDS[@]}"; do
    if ! kubectl get crd "$crd" &> /dev/null; then
        MISSING_CRDS+=("$crd")
    fi
done

if [ ${#MISSING_CRDS[@]} -ne 0 ]; then
    echo "❌ Missing CRDs:"
    printf '   %s\n' "${MISSING_CRDS[@]}"
    exit 1
fi

echo "✅ All required CRDs are installed"

# Validate CRD scopes
echo "🔍 Validating CRD scopes..."

# Check v1 CRDs are cluster-scoped
V1_SCOPE=$(kubectl get crd buckets.backblaze.crossplane.io -o jsonpath='{.spec.scope}')
if [ "$V1_SCOPE" != "Cluster" ]; then
    echo "❌ v1 Bucket CRD should be cluster-scoped, but is: $V1_SCOPE"
    exit 1
fi

# Check v1beta1 CRDs are namespaced
V1BETA1_SCOPE=$(kubectl get crd buckets.bucket.backblaze.m.crossplane.io -o jsonpath='{.spec.scope}')
if [ "$V1BETA1_SCOPE" != "Namespaced" ]; then
    echo "❌ v1beta1 Bucket CRD should be namespaced, but is: $V1BETA1_SCOPE"
    exit 1
fi

echo "✅ CRD scopes are correct (v1=Cluster, v1beta1=Namespaced)"

# Validate API groups
echo "🏷️  Validating API groups..."

V1_GROUP=$(kubectl get crd buckets.backblaze.crossplane.io -o jsonpath='{.spec.group}')
V1BETA1_GROUP=$(kubectl get crd buckets.bucket.backblaze.m.crossplane.io -o jsonpath='{.spec.group}')

if [ "$V1_GROUP" != "backblaze.crossplane.io" ]; then
    echo "❌ v1 API group should be 'backblaze.crossplane.io', but is: $V1_GROUP"
    exit 1
fi

if [ "$V1BETA1_GROUP" != "bucket.backblaze.m.crossplane.io" ]; then
    echo "❌ v1beta1 API group should be 'bucket.backblaze.m.crossplane.io', but is: $V1BETA1_GROUP"
    exit 1
fi

echo "✅ API groups are correct (v1 without .m., v1beta1 with .m.)"

# Test resource creation (dry-run)
echo "🧪 Testing resource creation capabilities..."

# Create a test namespace for v1beta1 resources
kubectl create namespace backblaze-test-validation --dry-run=client -o yaml > /dev/null 2>&1

# Test v1 cluster-scoped resource
cat <<EOF | kubectl apply --dry-run=server -f - > /dev/null
apiVersion: bucket.backblaze.crossplane.io/v1
kind: Bucket
metadata:
  name: test-v1-bucket
spec:
  providerConfigRef:
    name: default
  forProvider:
    bucketName: test-v1-bucket
    region: us-west-001
    bucketType: allPrivate
EOF

if [ $? -ne 0 ]; then
    echo "❌ Failed to validate v1 cluster-scoped resource creation"
    exit 1
fi

# Test v1beta1 namespaced resource
cat <<EOF | kubectl apply --dry-run=server -f - > /dev/null
apiVersion: bucket.backblaze.m.crossplane.io/v1beta1
kind: Bucket
metadata:
  name: test-v1beta1-bucket
  namespace: backblaze-test-validation
spec:
  providerConfigRef:
    name: default
  forProvider:
    bucketName: test-v1beta1-bucket
    region: us-west-001
    bucketType: allPrivate
EOF

if [ $? -ne 0 ]; then
    echo "❌ Failed to validate v1beta1 namespaced resource creation"
    exit 1
fi

echo "✅ Resource creation validation passed"

# Validate provider package information
echo "📋 Checking provider package details..."
PACKAGE_IMAGE=$(kubectl get provider.pkg.crossplane.io provider-backblaze -o jsonpath='{.spec.package}')
CURRENT_IMAGE=$(kubectl get provider.pkg.crossplane.io provider-backblaze -o jsonpath='{.status.currentIdentifier}')

if [[ ! "$PACKAGE_IMAGE" =~ v0\.9\.[0-9]+ ]]; then
    echo "⚠️  Warning: Provider package doesn't appear to be v0.9.x with v2 support: $PACKAGE_IMAGE"
fi

echo "📦 Provider package: $PACKAGE_IMAGE"
echo "🔄 Current package: $CURRENT_IMAGE"

# Check provider logs for errors
echo "📝 Checking provider logs for errors..."
POD_NAME=$(kubectl get pods -n crossplane-system -l app.kubernetes.io/name=provider-backblaze -o jsonpath='{.items[0].metadata.name}')

if [ -n "$POD_NAME" ]; then
    ERROR_COUNT=$(kubectl logs -n crossplane-system "$POD_NAME" --tail=100 | grep -i error | wc -l)
    if [ "$ERROR_COUNT" -gt 0 ]; then
        echo "⚠️  Warning: Found $ERROR_COUNT error messages in provider logs"
        echo "   Run: kubectl logs -n crossplane-system $POD_NAME"
    else
        echo "✅ No recent errors in provider logs"
    fi
fi

# Summary
echo ""
echo "🎉 Provider-backblaze Crossplane v2 validation completed successfully!"
echo ""
echo "📊 Summary:"
echo "   ✅ Provider is healthy and running"
echo "   ✅ All required CRDs installed (v1 + v1beta1)"
echo "   ✅ Proper scoping (v1=Cluster, v1beta1=Namespaced)"
echo "   ✅ Correct API groups (v2 uses .m. pattern)"
echo "   ✅ Resource creation validation passed"
echo ""
echo "🚀 The provider now supports both:"
echo "   📍 v1 APIs: cluster-scoped (legacy compatibility)"
echo "   📍 v1beta1 APIs: namespaced (Crossplane v2 recommended)"
echo ""
echo "📚 Use v1beta1 APIs for new deployments to benefit from:"
echo "   • Namespace isolation and multi-tenancy"
echo "   • Better RBAC control"
echo "   • Crossplane v2 native features"
echo ""
echo "✨ Migration successful! Provider-backblaze now fully supports Crossplane v2."