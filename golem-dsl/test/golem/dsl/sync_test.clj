(ns golem.dsl.sync-test
  (:require [clojure.test :refer :all]
            [golem.dsl.sync :as sync]
            [clojure.java.io :as io]))

(deftest project-state-yaml-test
  (let [dsl-state {:goal "add auth"
                   :plan [{:step 1 :desc "implement"}]
                   :code {:files ["auth.go"]}
                   :test-results {:status :pass}}
        yaml-str (sync/project-state-yaml dsl-state "build-feature" "building")]
    (is (.contains yaml-str "phase: building"))
    (is (.contains yaml-str "current_focus: add auth"))))

(deftest write-state-yaml-test
  (let [dir (System/getProperty "java.io.tmpdir")
        path (str dir "/golem-test-" (System/currentTimeMillis) "/state.yaml")]
    (sync/write-state-yaml! path {:goal "test"} "build-feature" "building")
    (is (.exists (io/file path)))
    (let [content (slurp path)]
      (is (.contains content "phase: building"))
      (is (.contains content "current_focus: test")))
    ;; cleanup
    (.delete (io/file path))
    (.delete (.getParentFile (io/file path)))))
