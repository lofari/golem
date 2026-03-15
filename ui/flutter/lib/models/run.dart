class EngineEvent {
  final String type;
  final DateTime timestamp;
  final String? step;
  final String? stepType;
  final String? status;
  final int? durationMs;
  final String? agent;
  final String? goal;
  final String? runId;
  final String? predicate;
  final int? iteration;
  final int? max;
  final String? reason;
  final String? errorType;
  final String? action;
  final int? attempt;

  const EngineEvent({
    required this.type,
    required this.timestamp,
    this.step,
    this.stepType,
    this.status,
    this.durationMs,
    this.agent,
    this.goal,
    this.runId,
    this.predicate,
    this.iteration,
    this.max,
    this.reason,
    this.errorType,
    this.action,
    this.attempt,
  });

  factory EngineEvent.fromJson(Map<String, dynamic> json) {
    return EngineEvent(
      type: json['type'] as String,
      timestamp: DateTime.parse(json['timestamp'] as String),
      step: json['step'] as String?,
      stepType: json['step-type'] as String?,
      status: json['status'] as String?,
      durationMs: (json['duration-ms'] as num?)?.toInt(),
      agent: json['agent'] as String?,
      goal: json['goal'] as String?,
      runId: json['run-id'] as String?,
      predicate: json['predicate'] as String?,
      iteration: (json['iteration'] as num?)?.toInt(),
      max: (json['max'] as num?)?.toInt(),
      reason: json['reason'] as String?,
      errorType: json['error-type'] as String?,
      action: json['action'] as String?,
      attempt: (json['attempt'] as num?)?.toInt(),
    );
  }
}

class RunInfo {
  final String runId;
  final String agentName;
  final String goal;
  final String projectId;
  final String projectName;
  final String status; // running, success, error
  final DateTime startedAt;
  final Duration? duration;
  final String? prUrl;
  final String? branch;
  final String? haltReason;
  final List<StepProgress> steps;

  const RunInfo({
    required this.runId,
    required this.agentName,
    required this.goal,
    required this.projectId,
    required this.projectName,
    required this.status,
    required this.startedAt,
    this.duration,
    this.prUrl,
    this.branch,
    this.haltReason,
    required this.steps,
  });
}

class StepProgress {
  final String name;
  final String type; // agentic, builtin, shell
  final String status; // pending, running, success, error, skipped
  final Duration? duration;
  final DateTime? startedAt;
  final int toolCallCount;

  const StepProgress({
    required this.name,
    required this.type,
    required this.status,
    this.duration,
    this.startedAt,
    this.toolCallCount = 0,
  });
}
