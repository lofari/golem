(ns golem.dsl.cli.main
  "CLI entry point for golem-dsl."
  (:require [golem.dsl.engine.core :as engine]
            [golem.dsl.engine.snapshot :as snapshot]
            [golem.dsl.session.claude :as claude]
            [golem.dsl.registry :as registry]
            [clojure.edn :as edn]
            [clojure.java.io :as io]))

(defn- load-agent-file [path]
  (load-file path))

(defn- parse-state-arg [args]
  (cond
    (some #(= "--goal" %) args)
    (let [idx (.indexOf (vec args) "--goal")]
      {:goal (nth args (inc idx))})

    (some #(= "--state" %) args)
    (let [idx (.indexOf (vec args) "--state")]
      (edn/read-string (slurp (nth args (inc idx)))))

    :else {}))

(defn cmd-run [args]
  (let [agent-file (first args)
        _ (load-agent-file agent-file)
        agent-name (keyword (second args))
        initial-state (parse-state-arg (drop 2 args))
        adapter (claude/make-adapter {:golem-binary "golem"})
        result (engine/run agent-name initial-state
                          {:adapter adapter
                           :base-dir "."})]
    (println "Run complete:" (:run-id result))
    (println "Final status:" (:status result))
    (println "State version:" (:version result))))

(defn cmd-compile [args]
  (let [agent-file (first args)]
    (load-agent-file agent-file)
    (doseq [[name agent] (registry/all-agents)]
      (println "Agent:" name)
      (println "Nodes:" (count (:nodes agent)))
      (println "Contracts valid:" (nil? (registry/validate name)))
      (println))))

(defn cmd-inspect [args]
  (let [target (first args)]
    (if-let [agent (registry/get-agent (keyword target))]
      (do (println "Agent:" (:name agent))
          (println "\nNodes:")
          (doseq [n (:nodes agent)]
            (println " " (:id n) "->" (:primitive n)
                     "reads:" (get-in n [:contract :reads])
                     "writes:" (get-in n [:contract :writes])))
          (println "\nEdges:")
          (doseq [e (:edges agent)]
            (println " " e)))
      ;; Try as run ID
      (let [version (when (second args) (Integer/parseInt (subs (second args) 2)))
            state (snapshot/load-state "." (keyword target) (or version 0))]
        (if state
          (clojure.pprint/pprint state)
          (println "Not found:" target))))))

(defn cmd-runs [_args]
  (let [runs-dir (io/file "." "runs")]
    (if (.exists runs-dir)
      (doseq [f (sort (.listFiles runs-dir))]
        (when (.isDirectory f)
          (println (.getName f))))
      (println "No runs found."))))

(defn -main [& args]
  (let [cmd (first args)
        rest-args (rest args)]
    (case cmd
      "run"     (cmd-run rest-args)
      "compile" (cmd-compile rest-args)
      "inspect" (cmd-inspect rest-args)
      "runs"    (cmd-runs rest-args)
      (do (println "Usage: golem-dsl <command> [args]")
          (println "Commands: run, compile, inspect, runs")))))
