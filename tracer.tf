# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2025 Datadog, Inc.

locals {
  apm_enabled              = var.datadog_apm_instrumentation != null
  tracer_volume_name       = "datadog-tracer"
  tracer_volume_mount_path = "/datadog-lib"
  tracer_init_name         = "datadog-tracer-copy"
  injection_mode_tag       = "_dd.injection.mode:serverless-single-lang"
  tracer_libc              = local.apm_enabled ? var.datadog_apm_instrumentation.tracer_libc : null
  php_loader_dir = local.apm_enabled ? (
    "${local.tracer_volume_mount_path}/${local.tracer_libc == "musl" ? "linux-musl" : "linux-gnu"}/loader"
  ) : null

  # Fragment-aware loader environment from the shared datadog-ci SSI model.
  apm_env_fragments_by_language = {
    java = [{
      name                   = "JAVA_TOOL_OPTIONS"
      value                  = "-javaagent:${local.tracer_volume_mount_path}/dd-java-agent.jar -XX:+IgnoreUnrecognizedVMOptions"
      mode                   = "append"
      separator              = " "
      preserve_leading_empty = false
      max_length             = null
    }]
    js = [{
      name                   = "NODE_OPTIONS"
      value                  = "--require ${local.tracer_volume_mount_path}/node_modules/dd-trace/init.js"
      mode                   = "append"
      separator              = " "
      preserve_leading_empty = false
      max_length             = null
    }]
    dotnet = [
      {
        name                   = "CORECLR_ENABLE_PROFILING"
        value                  = "1"
        mode                   = "set-if-absent"
        separator              = null
        preserve_leading_empty = false
        max_length             = null
      },
      {
        name                   = "CORECLR_PROFILER"
        value                  = "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}"
        mode                   = "set-if-absent"
        separator              = null
        preserve_leading_empty = false
        max_length             = null
      },
      {
        name                   = "CORECLR_PROFILER_PATH"
        value                  = "${local.tracer_volume_mount_path}/Datadog.Trace.ClrProfiler.Native.so"
        mode                   = "set-if-absent"
        separator              = null
        preserve_leading_empty = false
        max_length             = null
      },
      {
        name                   = "DD_DOTNET_TRACER_HOME"
        value                  = local.tracer_volume_mount_path
        mode                   = "set-if-absent"
        separator              = null
        preserve_leading_empty = false
        max_length             = null
      },
      {
        name                   = "LD_PRELOAD"
        value                  = "${local.tracer_volume_mount_path}/continuousprofiler/Datadog.Linux.ApiWrapper.x64.so"
        mode                   = "prepend"
        separator              = " "
        preserve_leading_empty = false
        max_length             = 1024
      },
    ]
    python = [{
      name                   = "PYTHONPATH"
      value                  = local.tracer_volume_mount_path
      mode                   = "append"
      separator              = ":"
      preserve_leading_empty = false
      max_length             = null
    }]
    ruby = [{
      name                   = "RUBYOPT"
      value                  = "-r${local.tracer_volume_mount_path}/auto_inject"
      mode                   = "prepend"
      separator              = " "
      preserve_leading_empty = false
      max_length             = null
    }]
    php = [
      {
        name                   = "PHP_INI_SCAN_DIR"
        value                  = local.php_loader_dir
        mode                   = "append"
        separator              = ":"
        preserve_leading_empty = true
        max_length             = null
      },
      {
        name                   = "DD_LOADER_PACKAGE_PATH"
        value                  = local.tracer_volume_mount_path
        mode                   = "set-if-absent"
        separator              = null
        preserve_leading_empty = false
        max_length             = null
      },
    ]
  }
  apm_env_fragments    = local.apm_enabled ? local.apm_env_fragments_by_language[var.datadog_apm_instrumentation.language] : []
  apm_loader_env_names = [for fragment in local.apm_env_fragments : fragment.name]
  apm_merged_env_names = concat(local.apm_loader_env_names, ["DD_TAGS"])

  apm_configured_container_name = local.apm_enabled ? try(var.datadog_apm_instrumentation.container_name, null) : null
  apm_matching_container_indexes = local.apm_enabled && local.apm_configured_container_name != null ? [
    for index, container in local.containers_without_sidecar : index
    if container.name == local.apm_configured_container_name
  ] : []
  apm_target_container_index = !local.apm_enabled ? null : (
    local.apm_configured_container_name != null ? try(local.apm_matching_container_indexes[0], null) : (
      length(local.containers_without_sidecar) == 1 ? 0 : null
    )
  )
  apm_target_container = local.apm_target_container_index != null ? local.containers_without_sidecar[local.apm_target_container_index] : null

  apm_target_literal_env = local.apm_target_container == null ? {} : {
    for env in coalesce(local.apm_target_container.env, []) : env.name => env.value
    if env.secret_name == null && env.value != null && length([
      for candidate in coalesce(local.apm_target_container.env, []) : candidate
      if candidate.name == env.name
    ]) == 1
  }
  apm_merged_loader_env = {
    for fragment in local.apm_env_fragments : fragment.name => (
      fragment.mode == "set-if-absent" ? (
        try(local.apm_target_literal_env[fragment.name], "") != "" ?
        try(local.apm_target_literal_env[fragment.name], null) : fragment.value
        ) : (
        try(local.apm_target_literal_env[fragment.name], null) != null && strcontains(
          "${fragment.separator}${try(local.apm_target_literal_env[fragment.name], "")}${fragment.separator}",
          "${fragment.separator}${fragment.value}${fragment.separator}",
          ) ? try(local.apm_target_literal_env[fragment.name], null) : (
          try(local.apm_target_literal_env[fragment.name], "") == "" ?
          (fragment.preserve_leading_empty ? "${fragment.separator}${fragment.value}" : fragment.value) : (
            fragment.mode == "append" ?
            "${try(local.apm_target_literal_env[fragment.name], "")}${fragment.separator}${fragment.value}" :
            "${fragment.value}${fragment.separator}${try(local.apm_target_literal_env[fragment.name], "")}"
          )
        )
      )
    )
  }
  apm_managed_secret_env_names = local.apm_target_container == null ? [] : distinct([
    for env in coalesce(local.apm_target_container.env, []) : env.name
    if env.secret_name != null && contains(local.apm_merged_env_names, env.name)
  ])
  apm_managed_duplicate_env_names = local.apm_target_container == null ? [] : distinct([
    for name in local.apm_merged_env_names : name
    if length([for env in coalesce(local.apm_target_container.env, []) : env if env.name == name]) > 1
  ])
  apm_loader_set_if_absent_conflicts = [
    for fragment in local.apm_env_fragments : fragment.name
    if fragment.mode == "set-if-absent" &&
    try(local.apm_target_literal_env[fragment.name], "") != "" &&
    try(local.apm_target_literal_env[fragment.name], null) != fragment.value
  ]
  apm_loader_env_exceeding_max_length = [
    for fragment in local.apm_env_fragments : fragment.name
    if fragment.max_length == null ? false : length(local.apm_merged_loader_env[fragment.name]) > fragment.max_length
  ]

  apm_current_dd_tags = local.apm_enabled && local.apm_target_container != null ? (
    try(local.apm_target_literal_env["DD_TAGS"], null) != null ?
    try(local.apm_target_literal_env["DD_TAGS"], null) : try(local.shared_env_vars["DD_TAGS"], "")
  ) : ""
  apm_merged_dd_tags = strcontains(
    ",${local.apm_current_dd_tags},",
    ",${local.injection_mode_tag},",
    ) ? local.apm_current_dd_tags : (
    local.apm_current_dd_tags == "" ? local.injection_mode_tag : "${local.injection_mode_tag},${local.apm_current_dd_tags}"
  )

  volumes_without_tracer_volume = local.apm_enabled ? [
    for volume in local.volumes_without_shared_volume : volume
    if volume.name != local.tracer_volume_name
  ] : local.volumes_without_shared_volume
  apm_ignored_tracer_volume_mounts = !local.apm_enabled ? [] : flatten([
    for index, container in local.containers_without_sidecar : [
      for volume_mount in coalesce(container.volume_mounts, []) : volume_mount
      if volume_mount.name == local.tracer_volume_name || (
        index == local.apm_target_container_index && volume_mount.path == local.tracer_volume_mount_path
      )
    ]
  ])

  init_containers_without_tracer_copy = local.apm_enabled ? [
    for container in coalesce(var.template.init_container, []) : container
    if container.name != local.tracer_init_name
  ] : coalesce(var.template.init_container, [])
  tracer_volume_mount = {
    name     = local.tracer_volume_name
    path     = local.tracer_volume_mount_path
    sub_path = null
  }
  tracer_copy_init_container = local.apm_enabled ? {
    name    = local.tracer_init_name
    image   = "datadoghq.azurecr.io/dd-lib-${var.datadog_apm_instrumentation.language}-init:${var.datadog_apm_instrumentation.tracer_version}"
    command = ["/datadog-init/copy-lib.sh"]
    args    = [local.tracer_volume_mount_path]
    cpu     = 0.25
    memory  = "0.5Gi"
    env     = null
    volume_mounts = [
      local.tracer_volume_mount,
    ]
  } : null
}

check "apm_target_container_known" {
  assert {
    condition = (
      !local.apm_enabled || local.apm_configured_container_name == null ||
      length(local.apm_matching_container_indexes) > 0
    )
    error_message = "datadog_apm_instrumentation.container_name is \"${coalesce(local.apm_configured_container_name, "null")}\", but no application container has that name. Available application containers: ${join(", ", [for container in local.containers_without_sidecar : container.name])}."
  }
}

check "apm_target_container_not_ambiguous" {
  assert {
    condition = (
      !local.apm_enabled || local.apm_configured_container_name != null ||
      length(local.containers_without_sidecar) <= 1
    )
    error_message = "Multiple application containers are available: ${join(", ", [for container in local.containers_without_sidecar : container.name])}. Set datadog_apm_instrumentation.container_name to the container to instrument."
  }
}

check "apm_managed_env_not_secret_backed" {
  assert {
    condition     = !local.apm_enabled || length(local.apm_managed_secret_env_names) == 0
    error_message = "SSI environment variable(s) on the target container use secret references and cannot be safely extended: ${join(", ", local.apm_managed_secret_env_names)}. Set them to literal values or remove them before enabling datadog_apm_instrumentation."
  }
}

check "apm_managed_env_not_duplicated" {
  assert {
    condition     = !local.apm_enabled || length(local.apm_managed_duplicate_env_names) == 0
    error_message = "SSI environment variable(s) appear more than once on the target container and cannot be safely modified: ${join(", ", local.apm_managed_duplicate_env_names)}. Remove the duplicates before enabling datadog_apm_instrumentation."
  }
}

check "apm_loader_env_set_if_absent_compatible" {
  assert {
    condition     = !local.apm_enabled || length(local.apm_loader_set_if_absent_conflicts) == 0
    error_message = "SSI loader environment variable(s) conflict with required tracer values: ${join(", ", local.apm_loader_set_if_absent_conflicts)}. Remove them or set them to the required Datadog tracer values."
  }
}

check "apm_loader_env_within_max_length" {
  assert {
    condition     = !local.apm_enabled || length(local.apm_loader_env_exceeding_max_length) == 0
    error_message = "SSI loader environment variable(s) exceed their maximum length after adding the tracer fragment: ${join(", ", local.apm_loader_env_exceeding_max_length)}. Shorten the existing values."
  }
}

check "apm_tracer_volume_already_exists" {
  assert {
    condition = !local.apm_enabled || length(coalesce(var.template.volume, [])) == length([
      for volume in coalesce(var.template.volume, []) : volume if volume.name != local.tracer_volume_name
    ])
    error_message = "A volume named \"${local.tracer_volume_name}\" already exists in template.volume. This module will replace it with the EmptyDir volume required by datadog_apm_instrumentation."
  }
}

check "apm_tracer_init_container_already_exists" {
  assert {
    condition     = !local.apm_enabled || length(coalesce(var.template.init_container, [])) == length(local.init_containers_without_tracer_copy)
    error_message = "An init container named \"${local.tracer_init_name}\" already exists in template.init_container. This module will replace it with the tracer copy init container required by datadog_apm_instrumentation."
  }
}

check "apm_tracer_volume_mount_already_exists" {
  assert {
    condition     = !local.apm_enabled || length(local.apm_ignored_tracer_volume_mounts) == 0
    error_message = "Application containers have volume mounts that conflict with the managed tracer volume: ${join(", ", [for mount in local.apm_ignored_tracer_volume_mounts : "${mount.name}:${mount.path}"])}. This module will remove them and mount ${local.tracer_volume_name}:${local.tracer_volume_mount_path} only on the target container."
  }
}

output "ignored_init_containers" {
  description = "List of init containers that are replaced by the module-managed tracer copy init container."
  value = [
    for container in coalesce(var.template.init_container, []) : container
    if !contains(local.init_containers_without_tracer_copy, container)
  ]
}
