class ProcessInfo {
  final String id;
  final String command;
  final String status;
  final String startedAt;
  final int? pid;

  ProcessInfo({
    required this.id,
    required this.command,
    required this.status,
    required this.startedAt,
    this.pid,
  });

  factory ProcessInfo.fromJson(Map<String, dynamic> json) => ProcessInfo(
        id: json['id'] as String? ?? '',
        command: json['command'] as String? ?? '',
        status: json['status'] as String? ?? '',
        startedAt: json['startedAt'] as String? ?? '',
        pid: json['pid'] as int?,
      );
}

class LaunchRequest {
  final String command;
  final LaunchConfig config;

  LaunchRequest({required this.command, required this.config});

  Map<String, dynamic> toJson() => {
        'command': command,
        'config': config.toJson(),
      };
}

class LaunchConfig {
  final int? maxIterations;
  final int? maxToolCalls;
  final String? model;
  final String? task;
  final bool sandbox;
  final bool mcp;
  final int? parallel;

  LaunchConfig({
    this.maxIterations,
    this.maxToolCalls,
    this.model,
    this.task,
    this.sandbox = false,
    this.mcp = true,
    this.parallel,
  });

  Map<String, dynamic> toJson() {
    final map = <String, dynamic>{};
    if (maxIterations != null) map['maxIterations'] = maxIterations;
    if (maxToolCalls != null) map['maxToolCalls'] = maxToolCalls;
    if (model != null && model!.isNotEmpty) map['model'] = model;
    if (task != null && task!.isNotEmpty) map['task'] = task;
    if (sandbox) map['sandbox'] = true;
    if (mcp) map['mcp'] = true;
    if (parallel != null && parallel! > 1) map['parallel'] = parallel;
    return map;
  }
}

class GolemConfig {
  int maxIterations;
  int maxToolCalls;
  bool verbose;
  bool sandbox;
  bool mcp;
  int parallel;
  String model;

  GolemConfig({
    this.maxIterations = 20,
    this.maxToolCalls = 200,
    this.verbose = false,
    this.sandbox = false,
    this.mcp = true,
    this.parallel = 1,
    this.model = '',
  });

  factory GolemConfig.fromJson(Map<String, dynamic> json) => GolemConfig(
        maxIterations: json['max-iterations'] as int? ?? 20,
        maxToolCalls: json['max-tool-calls'] as int? ?? 200,
        verbose: json['verbose'] as bool? ?? false,
        sandbox: json['sandbox'] as bool? ?? false,
        mcp: json['mcp'] as bool? ?? true,
        parallel: json['parallel'] as int? ?? 1,
        model: json['model'] as String? ?? '',
      );

  Map<String, dynamic> toJson() => {
        'max-iterations': maxIterations,
        'max-tool-calls': maxToolCalls,
        'verbose': verbose,
        'sandbox': sandbox,
        'mcp': mcp,
        'parallel': parallel,
        'model': model,
      };
}
