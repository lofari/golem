(ns golem.dsl.engine.state-test
  (:require [clojure.test :refer [deftest is testing]]
            [golem.dsl.engine.state :as state]))

(deftest apply-writes-enforces-contract
  (testing "accepts declared writes"
    (let [result (state/apply-writes
                  {:goal "test" :plan []}
                  {:code {:files ["a.go"]} :test-results {:status :pass}}
                  [:code :test-results])]
      (is (= {:files ["a.go"]} (:code result)))
      (is (= {:status :pass} (:test-results result)))))

  (testing "rejects undeclared writes"
    (is (thrown? Exception
          (state/apply-writes
           {:goal "test"}
           {:code {} :extra-key "bad"}
           [:code])))))

(deftest validate-reads-checks-state
  (testing "returns nil when all reads present"
    (is (nil? (state/validate-reads {:goal "x" :plan []} [:goal :plan]))))

  (testing "returns error for missing reads"
    (let [result (state/validate-reads {:goal "x"} [:goal :plan])]
      (is (some? result))
      (is (= [:plan] (:missing-keys result))))))

(deftest select-reads-extracts-keys
  (let [full-state {:goal "x" :plan [] :code {} :extra "y"}]
    (is (= {:goal "x" :plan []}
           (state/select-reads full-state [:goal :plan] nil)))
    (is (= {:goal "x" :plan [] :code {}}
           (state/select-reads full-state [:goal :plan] [:code :missing])))))
