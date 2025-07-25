#!/usr/bin/env python3
"""
Template processor for content-sources-backend deployment.yaml

This script reads the deployment.template.yaml file and env-variables.yaml file,
then combines them to generate the final deployment.yaml file with all
environment variables properly organized and deduplicated.
"""

import yaml
import sys
import os
from typing import Dict, List, Any


def load_yaml_file(file_path: str) -> Dict[str, Any]:
    """Load a YAML file and return its contents as a dictionary."""
    try:
        with open(file_path, 'r', encoding='utf-8') as file:
            return yaml.safe_load(file)
    except FileNotFoundError:
        print(f"Error: File {file_path} not found", file=sys.stderr)
        sys.exit(1)
    except yaml.YAMLError as e:
        print(f"Error parsing YAML file {file_path}: {e}", file=sys.stderr)
        sys.exit(1)


def format_env_vars(env_vars: List[Dict[str, Any]]) -> str:
    """Format environment variables list as a YAML string."""
    if not env_vars:
        return ""
    
    # Convert to YAML string
    yaml_str = yaml.dump(env_vars, default_flow_style=False, sort_keys=False)
    
    # Add proper indentation for the env block
    lines = yaml_str.split('\n')
    indented_lines = []
    for line in lines:
        if line.strip():  # Skip empty lines
            indented_lines.append('              ' + line)
    
    return '\n'.join(indented_lines).rstrip()


def process_template(template_content: str, env_vars: Dict[str, Any]) -> str:
    """Process the template content by replacing placeholders with environment variables."""
    
    # Get the common environment variables
    common_env = env_vars.get('common', [])
    service_specific_env = env_vars.get('service_specific', [])
    job_specific_env = env_vars.get('job_specific', {})
    
    # Format environment variables
    common_env_str = format_env_vars(common_env)
    service_specific_env_str = format_env_vars(service_specific_env)
    
    # Get job-specific environment variables
    process_repos_env = job_specific_env.get('process_repos', [])
    transform_pulp_logs_env = job_specific_env.get('transform_pulp_logs', [])
    
    process_repos_env_str = format_env_vars(process_repos_env)
    transform_pulp_logs_env_str = format_env_vars(transform_pulp_logs_env)
    
    # Replace placeholders in template
    processed_content = template_content
    
    # Replace common environment variables (add newline if not empty)
    if common_env_str:
        processed_content = processed_content.replace('{{ENV_COMMON}}', '\n' + common_env_str)
    else:
        processed_content = processed_content.replace('{{ENV_COMMON}}', '')
    
    # Replace service-specific environment variables (add newline if not empty)
    if service_specific_env_str:
        processed_content = processed_content.replace('{{ENV_SERVICE_SPECIFIC}}', '\n' + service_specific_env_str)
    else:
        processed_content = processed_content.replace('{{ENV_SERVICE_SPECIFIC}}', '')
    
    # Replace job-specific environment variables (add newline if not empty)
    if process_repos_env_str:
        processed_content = processed_content.replace('{{ENV_JOB_PROCESS_REPOS}}', '\n' + process_repos_env_str)
    else:
        processed_content = processed_content.replace('{{ENV_JOB_PROCESS_REPOS}}', '')
    
    if transform_pulp_logs_env_str:
        processed_content = processed_content.replace('{{ENV_JOB_TRANSFORM_PULP_LOGS}}', '\n' + transform_pulp_logs_env_str)
    else:
        processed_content = processed_content.replace('{{ENV_JOB_TRANSFORM_PULP_LOGS}}', '')
    
    return processed_content


def main():
    """Main function to process the template and generate deployment.yaml."""
    
    # Get script directory
    script_dir = os.path.dirname(os.path.abspath(__file__))
    
    # Define file paths
    template_file = os.path.join(script_dir, 'deployment.template.yaml')
    env_vars_file = os.path.join(script_dir, 'env-variables.yaml')
    output_file = os.path.join(os.path.dirname(script_dir), 'deployment.yaml')
    
    # Check if template file exists
    if not os.path.exists(template_file):
        print(f"Error: Template file {template_file} not found", file=sys.stderr)
        sys.exit(1)
    
    # Check if env-variables file exists
    if not os.path.exists(env_vars_file):
        print(f"Error: Environment variables file {env_vars_file} not found", file=sys.stderr)
        sys.exit(1)
    
    # Load files
    print(f"Loading template file: {template_file}")
    with open(template_file, 'r', encoding='utf-8') as file:
        template_content = file.read()
    
    print(f"Loading environment variables file: {env_vars_file}")
    env_vars = load_yaml_file(env_vars_file)
    
    # Process template
    print("Processing template...")
    processed_content = process_template(template_content, env_vars)
    
    # Write output
    print(f"Writing output to: {output_file}")
    with open(output_file, 'w', encoding='utf-8') as file:
        file.write("# DO NOT EDIT THIS FILE DIRECTLY\n")
        file.write("#   This yaml file is generated from deployment.template.yaml\n")
        file.write("#   and env-variables.yaml in deployments/build.\n")
        file.write("#   Run 'make deployment-generate' to regenerate\n")
        file.write(processed_content)
    
    print("Template processing completed successfully!")


if __name__ == "__main__":
    main() 
