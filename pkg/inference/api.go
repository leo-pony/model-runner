package inference

// ExperimentalEndpointsPrefix is used to prefix all /engines routes on the Docker
// socket while they are still in their experimental stage. This prefix doesn't
// apply to endpoints on model-runner.docker.internal.
const ExperimentalEndpointsPrefix = "/exp/vDD4.40"
