#!/usr/bin/env python3
"""
Test script for validating MLflow API functionality.

This script tests basic MLflow operations including:
- Creating experiments
- Creating runs
- Logging parameters and metrics
- Retrieving experiment and run data
"""

import sys
import argparse
import mlflow
from mlflow.tracking import MlflowClient


def test_mlflow_connection(tracking_uri):
    """Test basic connection to MLflow tracking server."""
    print(f"Testing connection to MLflow at {tracking_uri}...")
    mlflow.set_tracking_uri(tracking_uri)

    # Set a default workspace if workspaces are enabled
    try:
        workspace_name = "default"
        print(f"Setting workspace to '{workspace_name}'...")
        mlflow.set_workspace(workspace_name)
        print(f"✓ Workspace set to '{workspace_name}'")
    except Exception as e:
        print(f"  Note: Could not set workspace ({e}), continuing without workspace")

    client = MlflowClient()

    try:
        # Try to list experiments as a basic connectivity test
        experiments = client.search_experiments()
        print(f"✓ Successfully connected to MLflow. Found {len(experiments)} experiment(s)")
        return True
    except Exception as e:
        print(f"✗ Failed to connect to MLflow: {e}")
        return False


def test_create_experiment(client, experiment_name):
    """Test creating a new experiment."""
    print(f"\nTesting experiment creation: {experiment_name}...")

    try:
        # Delete experiment if it already exists (for idempotent tests)
        existing = client.get_experiment_by_name(experiment_name)
        if existing:
            print(f"  Experiment '{experiment_name}' already exists (ID: {existing.experiment_id})")
            experiment_id = existing.experiment_id
        else:
            experiment_id = client.create_experiment(experiment_name)
            print(f"✓ Created experiment '{experiment_name}' (ID: {experiment_id})")

        # Verify the experiment was created
        experiment = client.get_experiment(experiment_id)
        assert experiment.name == experiment_name, f"Expected name {experiment_name}, got {experiment.name}"
        print(f"✓ Verified experiment details")

        return experiment_id
    except Exception as e:
        print(f"✗ Failed to create experiment: {e}")
        raise


def test_create_run_with_data(client, experiment_id):
    """Test creating a run and logging parameters/metrics."""
    print(f"\nTesting run creation and data logging...")

    try:
        # Create a run
        run = client.create_run(experiment_id)
        run_id = run.info.run_id
        print(f"✓ Created run (ID: {run_id})")

        # Log parameters
        params = {
            "learning_rate": "0.01",
            "batch_size": "32",
            "optimizer": "adam"
        }
        for key, value in params.items():
            client.log_param(run_id, key, value)
        print(f"✓ Logged {len(params)} parameters")

        # Log metrics
        metrics = {
            "accuracy": 0.95,
            "loss": 0.05,
            "f1_score": 0.92
        }
        for key, value in metrics.items():
            client.log_metric(run_id, key, value)
        print(f"✓ Logged {len(metrics)} metrics")

        # Verify the data was logged
        run_data = client.get_run(run_id)
        assert len(run_data.data.params) == len(params), \
            f"Expected {len(params)} params, got {len(run_data.data.params)}"
        assert len(run_data.data.metrics) == len(metrics), \
            f"Expected {len(metrics)} metrics, got {len(run_data.data.metrics)}"
        print(f"✓ Verified run data was logged correctly")

        # Terminate the run
        client.set_terminated(run_id)
        print(f"✓ Terminated run successfully")

        return run_id
    except Exception as e:
        print(f"✗ Failed to create/log run: {e}")
        raise


def test_search_runs(client, experiment_id):
    """Test searching for runs in an experiment."""
    print(f"\nTesting run search...")

    try:
        runs = client.search_runs([experiment_id])
        assert len(runs) > 0, "Expected at least one run in the experiment"
        print(f"✓ Found {len(runs)} run(s) in experiment")

        # Verify run data
        for run in runs:
            print(f"  Run {run.info.run_id}: {len(run.data.params)} params, {len(run.data.metrics)} metrics")

        return True
    except Exception as e:
        print(f"✗ Failed to search runs: {e}")
        raise


def test_model_registry(client, model_name):
    """Test basic model registry operations."""
    print(f"\nTesting model registry: {model_name}...")

    try:
        # Check if model already exists
        try:
            existing_model = client.get_registered_model(model_name)
            print(f"  Model '{model_name}' already exists")
        except Exception:
            # Create a registered model
            client.create_registered_model(model_name)
            print(f"✓ Created registered model '{model_name}'")

        # Verify the model exists
        model = client.get_registered_model(model_name)
        assert model.name == model_name, f"Expected model name {model_name}, got {model.name}"
        print(f"✓ Verified model '{model_name}' exists in registry")

        return True
    except Exception as e:
        print(f"✗ Failed model registry test: {e}")
        raise


def main():
    """Main test execution."""
    parser = argparse.ArgumentParser(description='Test MLflow API functionality')
    parser.add_argument('--tracking-uri', required=True, help='MLflow tracking URI')
    parser.add_argument('--experiment-name', default='test-experiment',
                       help='Name for test experiment')
    parser.add_argument('--model-name', default='test-model',
                       help='Name for test model')
    args = parser.parse_args()

    print("=" * 80)
    print("MLflow API Integration Test")
    print("=" * 80)

    # Test connection
    if not test_mlflow_connection(args.tracking_uri):
        print("\n✗ Connection test failed. Exiting.")
        sys.exit(1)

    client = MlflowClient()

    try:
        # Test experiment creation
        experiment_id = test_create_experiment(client, args.experiment_name)

        # Test run creation and data logging
        run_id = test_create_run_with_data(client, experiment_id)

        # Test searching for runs
        test_search_runs(client, experiment_id)

        # Test model registry
        test_model_registry(client, args.model_name)

        print("\n" + "=" * 80)
        print("✓ All tests passed successfully!")
        print("=" * 80)
        sys.exit(0)

    except Exception as e:
        print("\n" + "=" * 80)
        print(f"✗ Test suite failed: {e}")
        print("=" * 80)
        sys.exit(1)


if __name__ == "__main__":
    main()
